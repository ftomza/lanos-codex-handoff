package fuel

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

var ErrRevision = errors.New("fuel state revision changed; refresh and retry")
var ErrCalibration = errors.New("invalid fuel calibration")

type Calibration struct {
	Token          string  `json:"preview_token"`
	MeasuredLiters float64 `json:"measured_liters"`
	Factor         float64 `json:"factor"`
	Previous       float64 `json:"previous_factor"`
	Liters         float64 `json:"adjusted_liters"`
	Revision       uint64  `json:"revision"`
}

func (c *Computer) preview(liters float64, confirmed bool, revision uint64) (Calibration, error) {
	if revision != c.state.Revision {
		return Calibration{}, ErrRevision
	}
	s := c.state
	if !confirmed || !bounded(liters, .01, 1000) || s.Incomplete || s.CoveredKM < 10 || s.BaseLiters <= 0 {
		return Calibration{}, fmt.Errorf("%w: require matching full-to-full interval, complete coverage and at least 10 km", ErrCalibration)
	}
	factor := liters / s.BaseLiters
	if !bounded(factor, .25, 4) {
		return Calibration{}, fmt.Errorf("%w: factor must be within 0.25..4", ErrCalibration)
	}
	return Calibration{MeasuredLiters: liters, Factor: factor, Previous: s.Factor, Liters: s.BaseLiters * factor, Revision: s.Revision}, nil
}
func (c *Computer) Preview(liters float64, confirmed bool, revision uint64) (Calibration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, err := c.preview(liters, confirmed, revision)
	if err != nil {
		return Calibration{}, err
	}
	p.Token = rand.Text()
	c.pending = &p
	return p, nil
}
func (c *Computer) commit(next State, now time.Time) error {
	if c.closed {
		return fmt.Errorf("fuel computer is closed")
	}
	next.Revision++
	next.SavedAt = now.UTC()
	if err := next.validate(); err != nil {
		return err
	}
	if err := c.store.Save(next); err != nil {
		c.saveErr = err
		return err
	}
	c.state = next
	c.pending = nil
	c.lastSave = now
	c.saveErr = nil
	c.dirty = false
	return nil
}
func (c *Computer) Reset(revision uint64, defaultFactor bool, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if revision != c.state.Revision {
		return ErrRevision
	}
	next := State{Version: 1, Source: c.state.Source, Fingerprint: c.state.Fingerprint, Method: c.method, Revision: c.state.Revision, ResetAt: now.UTC(), SavedAt: now.UTC(), Factor: c.state.Factor, PreviousFactor: c.state.PreviousFactor, Reasons: []string{}}
	if defaultFactor {
		next.Factor = 1
		next.PreviousFactor = new(c.state.Factor)
	}
	if err := c.commit(next, now); err != nil {
		return err
	}
	c.previous = nil
	c.emitDiagnostic(now, "reset")
	return nil
}
func (c *Computer) Apply(action string, liters float64, confirmed bool, revision uint64, token string, now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if revision != c.state.Revision {
		return ErrRevision
	}
	factor := 1.0
	switch action {
	case "apply":
		if !confirmed || c.pending == nil || c.pending.Token != token || c.pending.Revision != revision || c.pending.MeasuredLiters != liters || c.state.Incomplete {
			return fmt.Errorf("%w: fresh matching calibration preview required", ErrCalibration)
		}
		factor = c.pending.Factor
	case "undo":
		if c.state.PreviousFactor == nil {
			return fmt.Errorf("%w: no previous factor", ErrCalibration)
		}
		factor = *c.state.PreviousFactor
	case "default":
	default:
		return fmt.Errorf("%w: unknown action", ErrCalibration)
	}
	next := c.state.clone()
	next.PreviousFactor = new(next.Factor)
	next.Factor = factor
	if err := c.commit(next, now); err != nil {
		return err
	}
	c.emitDiagnostic(now, "calibration_"+action)
	return nil
}
