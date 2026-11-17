package fuel

import (
	"math"
	"slices"
	"testing"
	"time"
)

func TestDiagnosticUsesActualCalculationAndPreservesState(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	cfg := DefaultConfig()
	c.Observe(now, input(now, map[string]float64{"rpm": 2000, "map_kpa": 40, "iat_c": 20, "speed_kmh": 40}))
	d := c.Diagnostic(now)
	terms := d.Fuel.Intermediates
	if terms == nil || terms.TemperatureK == nil || *terms.TemperatureK != 293.15 || terms.AirGS == nil || terms.RawFlowLPH == nil {
		t.Fatalf("missing units/calculation: %+v", terms)
	}
	if math.Abs(*terms.RawFlowLPH-*terms.AirGS*3600/(cfg.AFR*cfg.Density)) > 1e-12 || *d.Fuel.FlowLPH != *terms.RawFlowLPH {
		t.Fatal("diagnostics disagree with estimator")
	}
	if d.Fuel.Quality != "DEGRADED" || !slices.Contains(d.Fuel.QualityReasons, "assumed_ve") {
		t.Fatal(d.Fuel)
	}
	*d.Snapshot.Signals["rpm"].Value = 0
	if *c.latest.Signals["rpm"].Value != 2000 {
		t.Fatal("mutable diagnostic")
	}
	if c.state.Version != 1 || d.Fingerprint != c.state.Fingerprint {
		t.Fatal("changed state format")
	}
	if c.Diagnostic(now.Add(4*time.Second)).Fuel.Quality != "UNAVAILABLE" {
		t.Fatal("stale quality")
	}
}

func TestIATDiagnosticsUseDistinctFreshMeasurements(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	for i := 0; i <= 300; i += 5 {
		at := now.Add(time.Duration(i) * time.Second)
		c.Observe(at, input(at, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": 20}))
	}
	if !slices.Contains(c.View(now.Add(300*time.Second)).QualityReasons, "iat_constant") {
		t.Fatal("constant IAT not detected")
	}
	at := now.Add(305 * time.Second)
	c.Observe(at, input(at, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": -40}))
	f := c.View(at)
	if f.Quality != "UNAVAILABLE" || !slices.Contains(f.QualityReasons, "iat_jump") || !slices.Contains(f.QualityReasons, "iat_out_of_range") {
		t.Fatalf("IAT diagnostics: %+v", f)
	}
	c2, _ := computer(t, now)
	sample := input(now, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": 20})
	for i := range 400 {
		c2.Observe(now.Add(time.Duration(i)*time.Millisecond), sample)
	}
	if slices.Contains(c2.View(now).QualityReasons, "iat_constant") {
		t.Fatal("duplicate samples counted")
	}
}

func TestDiagnosticObserverRecordsControlsAndColdStart(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	var rows []Diagnostic
	if err := c.SetDiagnosticObserver(time.Second, true, func(d Diagnostic) error { rows = append(rows, d); return nil }); err != nil {
		t.Fatal(err)
	}
	c.Observe(now, input(now, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": 20, "coolant_c": 50}))
	if !slices.Contains(c.View(now).QualityReasons, "iat_cold_mismatch") {
		t.Fatal("explicit cold annotation ignored")
	}
	c.Tick(now.Add(250 * time.Millisecond))
	if len(rows) != 1 {
		t.Fatalf("unexpected sample rate: %d", len(rows))
	}
	if err := c.Reset(c.state.Revision, false, now.Add(500*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].Reason != "reset" || rows[1].Fuel.ResetAt.Equal(rows[0].Fuel.ResetAt) {
		t.Fatalf("missing control boundary: %+v", rows)
	}
	c2, _ := computer(t, now)
	c2.Observe(now, input(now, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": 20, "coolant_c": 50}))
	if slices.Contains(c2.View(now).QualityReasons, "iat_cold_mismatch") {
		t.Fatal("cold start inferred")
	}
}

func TestObserverAlwaysEmitsGenerationAndAvailabilityEvents(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	var rows []Diagnostic
	if err := c.SetDiagnosticObserver(time.Second, false, func(d Diagnostic) error { rows = append(rows, d); return nil }); err != nil {
		t.Fatal(err)
	}
	c.Observe(now, drive(now, 3.6, 36))
	later := now.Add(2 * time.Second)
	s := drive(later, 3.6, 36)
	s.Generation = 2
	c.Observe(later, s)
	if rows[len(rows)-1].Kind != "event" || rows[len(rows)-1].Reason != "generation_changed" {
		t.Fatal("generation not recorded as event", rows)
	}
	later = later.Add(2 * time.Second)
	s = drive(later, 3.6, 36)
	s.Generation = 2
	v := s.Signals["fuel_rate_lph"]
	v.Value = nil
	v.Status = "temporarily_unavailable"
	s.Signals["fuel_rate_lph"] = v
	c.Observe(later, s)
	if rows[len(rows)-1].Kind != "event" {
		t.Fatal("calculation loss not recorded as event")
	}
}

func TestStartupIATObservationsRemainDiagnosableBeforeLiveReady(t *testing.T) {
	t.Parallel()
	now := time.Now()
	c, _ := computer(t, now)
	s := input(now, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": -40})
	s.Health = "connecting"
	c.Observe(now, s)
	later := now.Add(5 * time.Second)
	c.Observe(later, input(later, map[string]float64{"rpm": 900, "map_kpa": 35, "iat_c": 20}))
	if !slices.Contains(c.View(later).QualityReasons, "iat_jump") || !slices.Contains(c.View(later).QualityReasons, "iat_out_of_range") {
		t.Fatal("lost raw startup IAT evidence")
	}
}
