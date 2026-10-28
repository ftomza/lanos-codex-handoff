package fuel

import (
	"math"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func input(now time.Time, values map[string]float64) telemetry.Snapshot {
	s := telemetry.Snapshot{Source: telemetry.Simulator, Health: telemetry.Connected, CapturedAt: now, ReceivedAt: now, Generation: 1, Signals: make(map[string]telemetry.Signal, len(values))}
	for k, v := range values {
		s.Signals[k] = telemetry.Signal{Value: new(v), Support: "supported", Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000}
	}
	return s
}

func TestEstimatorUnitsPriorityAndLiveIAT(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := input(now, map[string]float64{"rpm": 900, "map_kpa": 40, "iat_c": 20, "speed_kmh": 0})
	cfg := DefaultConfig()
	r := Evaluate(cfg, s, "speed_density", now)
	want := 40e3 / (287.05 * 293.15) * .0015 * 900 / 120 * .75 * 1000 * 3600 / (14.7 * 745)
	if r.Flow == nil || math.Abs(*r.Flow-want) > 1e-10 {
		t.Fatalf("SI conversion: %+v want %v", r, want)
	}
	if Select(cfg, s) != "speed_density" {
		t.Fatal("wrong default method")
	}
	s.Signals["maf_g_s"] = s.Signals["map_kpa"]
	if Select(cfg, s) != "maf" {
		t.Fatal("MAF not preferred")
	}
	s.Signals["fuel_rate_lph"] = s.Signals["map_kpa"]
	if Select(cfg, s) != "direct" {
		t.Fatal("direct flow not preferred")
	}
	s.Source = telemetry.Live
	if got := Evaluate(cfg, s, "speed_density", now); got.Flow != nil || got.Reason != "unverified_iat" {
		t.Fatalf("unverified live IAT: %+v", got)
	}
	cfg.AllowUnverifiedIAT = true
	if got := Evaluate(cfg, s, "speed_density", now); got.Flow == nil || got.Confidence != "low" {
		t.Fatalf("explicit estimate: %+v", got)
	}
}

func TestInvalidAndExpiredInputsNeverBecomeZero(t *testing.T) {
	t.Parallel()
	now := time.Now()
	for _, tc := range []struct {
		key   string
		value float64
	}{{"iat_c", -40}, {"map_kpa", 0}, {"rpm", 7001}, {"rpm", math.NaN()}} {
		t.Run(tc.key+time.Duration(tc.value).String(), func(t *testing.T) {
			t.Parallel()
			s := input(now, map[string]float64{"rpm": 900, "map_kpa": 40, "iat_c": 20})
			sig := s.Signals[tc.key]
			sig.Value = new(tc.value)
			s.Signals[tc.key] = sig
			if r := Evaluate(DefaultConfig(), s, "speed_density", now); r.Flow != nil {
				t.Fatalf("invalid reading: %+v", r)
			}
		})
	}
	s := input(now, map[string]float64{"rpm": 0, "map_kpa": 40, "iat_c": 20})
	if r := Evaluate(DefaultConfig(), s, "speed_density", now); r.Flow == nil || *r.Flow != 0 {
		t.Fatal("fresh zero RPM lost")
	}
	if r := Evaluate(DefaultConfig(), s, "speed_density", now.Add(3*time.Second)); r.Flow != nil {
		t.Fatal("stale zero became available")
	}
	delete(s.Signals, "rpm")
	if r := Evaluate(DefaultConfig(), s, "speed_density", now); r.Flow != nil {
		t.Fatal("missing RPM became zero")
	}
}

func TestConfigRejectsNonfiniteAndUnsafeValues(t *testing.T) {
	t.Parallel()
	for _, c := range []Config{{}, {VE: math.NaN()}, {Model: "injector"}} {
		if c.Validate() == nil {
			t.Fatalf("invalid config accepted: %+v", c)
		}
	}
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
}
