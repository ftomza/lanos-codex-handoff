package fuel

import (
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

// Diagnostic is one coherent immutable observation of the same computer that
// maintains live totals. It is independent of the persistent fuel-state schema.
type SignalAge struct {
	CapturedMS *int64 `json:"captured_ms"`
	ReceivedMS *int64 `json:"received_ms"`
	Applicable bool   `json:"applicable"`
}

type Diagnostic struct {
	Ages        map[string]SignalAge `json:"signal_ages"`
	Kind        string               `json:"kind"`
	Reason      string               `json:"reason,omitempty"`
	At          time.Time            `json:"at"`
	ElapsedNS   int64                `json:"elapsed_ns"`
	Snapshot    telemetry.Snapshot   `json:"snapshot"`
	Fuel        telemetry.Fuel       `json:"fuel"`
	BaseLiters  float64              `json:"base_liters"`
	JointLiters float64              `json:"joint_liters"`
	Fingerprint string               `json:"fingerprint"`
}

type diagnosticObserver struct {
	sink       func(Diagnostic) error
	interval   time.Duration
	origin     time.Time
	last       time.Time
	health     telemetry.Health
	generation uint64
	available  bool
	err        error
}

// The sink must be nonblocking; a bounded recorder queue satisfies this contract.
func (c *Computer) SetDiagnosticObserver(interval time.Duration, cold bool, sink func(Diagnostic) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.diag.sink != nil || interval < 250*time.Millisecond || interval > 2*time.Second || sink == nil {
		return fmt.Errorf("invalid diagnostic observer configuration")
	}
	c.iat.cold = cold
	c.diag = diagnosticObserver{sink: sink, interval: interval, origin: c.diag.origin}
	return nil
}
func (c *Computer) Diagnostic(now time.Time) Diagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.diagnostic(now, "sample", "")
}
func (c *Computer) diagnostic(now time.Time, kind, reason string) Diagnostic {
	return Diagnostic{Ages: c.signalAges(now), Kind: kind, Reason: reason, At: now.UTC(), ElapsedNS: max(0, now.Sub(c.diag.origin).Nanoseconds()), Snapshot: c.latest.Clone(), Fuel: c.view(now), BaseLiters: c.state.BaseLiters, JointLiters: c.state.JointLiters, Fingerprint: c.state.Fingerprint}
}
func (c *Computer) emitDiagnostic(now time.Time, reason string) {
	d := &c.diag
	if d.sink == nil || d.err != nil {
		return
	}
	available := c.result.Flow != nil
	if reason == "" && !d.last.IsZero() {
		if d.generation != c.latest.Generation {
			reason = "generation_changed"
		} else if d.health != c.latest.Health || d.available != available {
			reason = "availability_changed"
		}
	}
	if reason == "" && !d.last.IsZero() && now.Sub(d.last) < d.interval {
		return
	}
	kind := "sample"
	if reason != "" {
		kind = "event"
	}
	d.available = available

	d.err = d.sink(c.diagnostic(now, kind, reason))
	d.last, d.health, d.generation = now, c.latest.Health, c.latest.Generation
}

func (c *Computer) signalAges(now time.Time) map[string]SignalAge {
	ages := make(map[string]SignalAge, len(c.latest.Signals))
	effective := c.latest.At(now, c.cfg.StaleAfter)
	for key, s := range c.latest.Signals {
		age := SignalAge{Applicable: effective.Signals[key].Status == "available" && effective.Signals[key].Value != nil}
		if !s.CapturedAt.IsZero() {
			age.CapturedMS = new(now.Sub(s.CapturedAt).Milliseconds())
		}
		if !s.ReceivedAt.IsZero() {
			age.ReceivedMS = new(now.Sub(s.ReceivedAt).Milliseconds())
		}
		ages[key] = age
	}
	return ages
}
