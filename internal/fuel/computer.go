package fuel

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type Computer struct {
	iat        iatTracker
	diag       diagnosticObserver
	pending    *Calibration
	mu         sync.Mutex
	cfg        Config
	state      State
	store      Store
	latest     telemetry.Snapshot
	method     string
	generation uint64
	previous   *endpoint
	result     Result
	speed      *float64
	speedUntil time.Time
	lastSave   time.Time
	dirty      bool
	saveErr    error
	closed     bool
	closeErr   error
}

func newComputer(cfg Config, source string, store Store, now time.Time, reset bool) (*Computer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if source != telemetry.Simulator && source != telemetry.Live {
		return nil, fmt.Errorf("unknown fuel source")
	}
	loaded, err := store.Load()
	if err != nil {
		return nil, err
	}
	c := &Computer{cfg: cfg, store: store, lastSave: now}
	c.diag.origin = now
	if loaded != nil {
		if err := loaded.validate(); err != nil {
			return nil, err
		}
		if !reset && (loaded.Source != source || loaded.Fingerprint != cfg.fingerprint()) {
			return nil, fmt.Errorf("fuel state source/config mismatch: use a separate --fuel-state or explicit --fuel-reset-state")
		}
		c.state = loaded.clone()
		c.state.mark("process_restart")
	}
	if loaded == nil || reset {
		c.state = State{Version: 1, Source: source, Fingerprint: cfg.fingerprint(), Revision: 1, ResetAt: now.UTC(), SavedAt: now.UTC(), Factor: 1, Reasons: []string{}}
		if loaded != nil {
			c.state.Revision = loaded.Revision + 1
		}
	}
	if err := c.save(now, true); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Computer) Observe(now time.Time, s telemetry.Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.latest = s.Clone()
	if s.Generation != c.generation {
		c.method = ""
		c.generation = s.Generation
	}
	if c.method == "" && s.Health == telemetry.Connected {
		c.method = Select(c.cfg, s)
	}
	c.iat.observe(now, s)
	c.advance(now)
	c.emitDiagnostic(now, "")
}
func (c *Computer) Tick(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.iat.observe(now, c.latest)
		c.advance(now)
		c.emitDiagnostic(now, "")
	}
}

func (c *Computer) save(now time.Time, force bool) error {
	if !force && (!c.dirty || now.Sub(c.lastSave) < 5*time.Second) {
		return nil
	}
	c.lastSave = now
	next := c.state.clone()
	next.SavedAt = now.UTC()
	c.saveErr = c.store.Save(next)
	if c.saveErr != nil {
		return c.saveErr
	}
	c.state = next
	c.dirty = false
	return nil
}
func (c *Computer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return c.closeErr
	}
	c.emitDiagnostic(time.Now(), "computer_close")
	c.closed = true
	c.closeErr = errors.Join(c.save(time.Now(), true), c.store.Close(), c.diag.err)
	return c.closeErr
}

func (c *Computer) View(now time.Time) telemetry.Fuel {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.view(now)
}
func (c *Computer) view(now time.Time) telemetry.Fuel {
	s := c.state
	f := telemetry.Fuel{Status: "unavailable", Reason: c.result.Reason, Method: c.method, Provenance: c.result.Provenance, Confidence: c.result.Confidence,
		FlowUntil: c.result.Until, SpeedUntil: c.speedUntil, LowSpeed: c.cfg.LowSpeed, Speed: c.speed,
		Liters: s.BaseLiters * s.Factor, DistanceKM: s.DistanceKM, CoveredKM: s.CoveredKM,
		ObservedSeconds: float64(s.ObservedNS) / 1e9, CoveredSeconds: float64(s.CoveredNS) / 1e9, GapSeconds: float64(s.GapNS) / 1e9,
		Incomplete: s.Incomplete, CoverageReasons: s.Reasons, ResetAt: s.ResetAt, SavedAt: s.SavedAt, Factor: s.Factor, PreviousFactor: s.PreviousFactor, Revision: s.Revision, Persistence: "saved",
		Settings: telemetry.FuelSettings{Displacement: c.cfg.Displacement, VE: c.cfg.VE, AFR: c.cfg.AFR, Density: c.cfg.Density, UnverifiedIAT: s.Source == telemetry.Live && c.method == "speed_density" && c.cfg.AllowUnverifiedIAT}}
	if c.result.Flow != nil {
		f.Status = "available"
		f.FlowLPH = new(*c.result.Flow * s.Factor)
	}
	if f.FlowLPH != nil && c.speed != nil && *c.speed >= c.cfg.LowSpeed {
		f.InstantL100 = new(*f.FlowLPH * 100 / *c.speed)
	}
	if s.CoveredKM >= .1 {
		f.AverageL100 = new(s.JointLiters * s.Factor * 100 / s.CoveredKM)
	}
	if c.dirty {
		f.Persistence = "pending"
	}
	if c.saveErr != nil {
		f.Persistence = "persistence_error"
		f.PersistenceError = c.saveErr.Error()
	}
	f.Intermediates = c.result.Terms.Clone()
	if f.Intermediates != nil {
		f.Intermediates.Factor = s.Factor
		f.Intermediates.AdjustedFlowLPH = f.FlowLPH
	}
	if c.diag.err != nil {
		f.DiagnosticsError = c.diag.err.Error()
	}
	f = f.At(now, c.latest.At(now, c.cfg.StaleAfter).Health)
	c.quality(&f)
	return f
}
