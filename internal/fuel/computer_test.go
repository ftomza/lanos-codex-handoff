package fuel

import (
	"errors"
	"math"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type memoryStore struct {
	state *State
	fail  error
	saves int
}

func (m *memoryStore) Load() (*State, error) {
	if m.state == nil {
		return nil, nil
	}
	s := m.state.clone()
	return &s, nil
}
func (m *memoryStore) Save(s State) error {
	if m.fail != nil {
		return m.fail
	}
	v := s.clone()
	m.state = &v
	m.saves++
	return nil
}
func (m *memoryStore) Close() error { return nil }
func computer(t *testing.T, now time.Time) (*Computer, *memoryStore) {
	t.Helper()
	m := &memoryStore{}
	c, err := newComputer(DefaultConfig(), telemetry.Simulator, m, now, false)
	if err != nil {
		t.Fatal(err)
	}
	return c, m
}
func drive(now time.Time, flow, speed float64) telemetry.Snapshot {
	return input(now, map[string]float64{"fuel_rate_lph": flow, "speed_kmh": speed})
}

func TestAccountingUsesMatchedIntervalsAndDoesNotBridgeGaps(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	c.Observe(now, drive(now, 3.6, 36))
	c.Observe(now.Add(time.Second), drive(now.Add(time.Second), 3.6, 36))
	v := c.View(now.Add(time.Second))
	if math.Abs(v.Liters-.001) > 1e-12 || math.Abs(v.DistanceKM-.01) > 1e-12 {
		t.Fatalf("units: %+v", v)
	}
	missing := drive(now.Add(2*time.Second), 3.6, 36)
	sig := missing.Signals["fuel_rate_lph"]
	sig.Value = nil
	sig.Status = "temporarily_unavailable"
	missing.Signals["fuel_rate_lph"] = sig
	c.Observe(now.Add(2*time.Second), missing)
	c.Observe(now.Add(3*time.Second), drive(now.Add(3*time.Second), 3.6, 36))
	c.Observe(now.Add(4*time.Second), drive(now.Add(4*time.Second), 3.6, 36))
	v = c.View(now.Add(4 * time.Second))
	if math.Abs(v.Liters-.002) > 1e-12 || math.Abs(v.DistanceKM-.04) > 1e-12 || math.Abs(v.CoveredKM-.02) > 1e-12 || !v.Incomplete {
		t.Fatalf("gap/denominator: %+v", v)
	}
	c.Tick(now.Add(4 * time.Second))
	if c.View(now.Add(4*time.Second)).Liters != v.Liters {
		t.Fatal("duplicate timestamp counted twice")
	}
	c.Tick(now.Add(20 * time.Second))
	if c.View(now.Add(20*time.Second)).FlowLPH != nil || c.View(now.Add(20*time.Second)).Liters != v.Liters {
		t.Fatal("silence extrapolated")
	}
}

func TestGenerationMethodChangeAndRestart(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, m := computer(t, now)
	c.Observe(now, drive(now, 3.6, 36))
	c.Observe(now.Add(time.Second), drive(now.Add(time.Second), 3.6, 36))
	next := drive(now.Add(2*time.Second), 3.6, 36)
	next.Generation = 2
	c.Observe(now.Add(2*time.Second), next)
	if v := c.View(now.Add(2 * time.Second)); v.Liters != .001 || !v.Incomplete {
		t.Fatalf("bridged reconnect: %+v", v)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := newComputer(DefaultConfig(), telemetry.Simulator, m, now.Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	v := restored.View(now.Add(time.Hour))
	if v.Liters != .001 || !v.Incomplete || v.FlowLPH != nil {
		t.Fatalf("restored anchor or lost trip: %+v", v)
	}
	changed := input(now.Add(time.Hour), map[string]float64{"maf_g_s": 2, "speed_kmh": 36})
	restored.Observe(now.Add(time.Hour), changed)
	if got := restored.View(now.Add(time.Hour)); got.Reason != "model_changed_reset_required" || got.FlowLPH != nil {
		t.Fatalf("mixed models: %+v", got)
	}
}

func TestDurableResetAndCalibrationReversible(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, m := computer(t, now)
	c.state.Method = "direct"
	c.method = "direct"
	c.state.BaseLiters = 1
	c.state.JointLiters = 1
	c.state.DistanceKM = 20
	c.state.CoveredKM = 20
	p, err := c.Preview(2, true, c.state.Revision)
	if err != nil || p.Factor != 2 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if err := c.Apply("apply", 2, true, c.state.Revision, p.Token, now); err != nil {
		t.Fatal(err)
	}
	if c.View(now).Liters != 2 || c.state.BaseLiters != 1 {
		t.Fatal("calibration changed raw liters")
	}
	if err := c.Apply("undo", 0, false, c.state.Revision, "", now); err != nil || c.View(now).Liters != 1 {
		t.Fatal("undo lost previous factor", err)
	}
	before := c.state.clone()
	m.fail = errors.New("disk unavailable")
	if err := c.Reset(c.state.Revision, false, now); err == nil || c.state.Revision != before.Revision || c.state.BaseLiters != 1 {
		t.Fatal("failed reset committed")
	}
	m.fail = nil
	if err := c.Reset(c.state.Revision, false, now); err != nil || c.View(now).Liters != 0 || m.state.BaseLiters != 0 {
		t.Fatal("reset not durable", err)
	}
	if _, err := c.Preview(2, true, before.Revision); err == nil {
		t.Fatal("stale mutation accepted")
	}
}

func TestCalibrationAppliesThePreviewedFactorWhileRecordingContinues(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	c.state.Method = "direct"
	c.method = "direct"
	c.state.BaseLiters = 1
	c.state.JointLiters = 1
	c.state.DistanceKM = 20
	c.state.CoveredKM = 20
	p, err := c.Preview(2, true, c.state.Revision)
	if err != nil {
		t.Fatal(err)
	}
	c.state.BaseLiters = 1.1
	c.state.JointLiters = 1.1
	if err := c.Apply("apply", 2, true, c.state.Revision, p.Token, now); err != nil {
		t.Fatal(err)
	}
	if c.state.Factor != p.Factor || c.View(now).Liters != 2.2 {
		t.Fatalf("preview drifted during recording: factor %v preview %v", c.state.Factor, p.Factor)
	}
}

func TestMAFDirectFormulasAndNoFuelCutGuess(t *testing.T) {
	t.Parallel()
	now := time.Now()
	cfg := DefaultConfig()
	s := input(now, map[string]float64{"maf_g_s": 14.7, "fuel_rate_lph": 4, "rpm": 3000, "map_kpa": 25, "iat_c": 20})
	if r := Evaluate(cfg, s, "maf", now); r.Flow == nil || math.Abs(*r.Flow-3600.0/745) > 1e-12 {
		t.Fatalf("MAF formula: %+v", r)
	}
	if r := Evaluate(cfg, s, "direct", now); r.Flow == nil || *r.Flow != 4 {
		t.Fatalf("direct rate: %+v", r)
	}
	if r := Evaluate(cfg, s, "speed_density", now); r.Flow == nil || *r.Flow <= 0 {
		t.Fatal("deceleration guessed as fuel cut")
	}
}

func TestTickRejectsExpiryBackwardsTimeAndModelReset(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, m := computer(t, now)
	c.Observe(now, drive(now, 3.6, 36))
	c.Tick(now.Add(time.Second))
	c.Tick(now.Add(-time.Second))
	v := c.View(now.Add(time.Second))
	if !v.Incomplete || v.Liters != .001 {
		t.Fatalf("reversal integration: %+v", v)
	}
	m.fail = errors.New("checkpoint failure")
	c.Tick(now.Add(10 * time.Second))
	if c.View(now.Add(10*time.Second)).Persistence != "persistence_error" {
		t.Fatal("checkpoint error disappeared")
	}
	if err := c.Close(); err == nil {
		t.Fatal("close swallowed persistence error")
	}
}
