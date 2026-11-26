package fuel

import (
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type fuelVector struct {
	Name, Reason  string
	RPM, MAP, IAT *float64
	Speed, Flow   *float64
	Live, Allow   bool
	Generation    uint64
	MS            int64
	Age           int64 `json:"age_ms"`
}

func (v fuelVector) snapshot(at time.Time) telemetry.Snapshot {
	values := make(map[string]float64, 4)
	for key, value := range map[string]*float64{"rpm": v.RPM, "map_kpa": v.MAP, "iat_c": v.IAT, "speed_kmh": v.Speed} {
		if value != nil {
			values[key] = *value
		}
	}
	s := input(at.Add(-time.Duration(v.Age)*time.Millisecond), values)
	s.CapturedAt, s.ReceivedAt = at, at
	if v.Live {
		s.Source = telemetry.Live
	}
	if v.Generation != 0 {
		s.Generation = v.Generation
	}
	return s
}

func TestAndroidFuelGolden(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/android-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Estimates, Trip []fuelVector
		Totals          map[string]float64
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.November, 23, 0, 0, 0, 0, time.UTC)
	for _, tc := range fixture.Estimates {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultConfig()
			cfg.AllowUnverifiedIAT = tc.Allow
			result := Evaluate(cfg, tc.snapshot(now), "speed_density", now)
			if tc.Flow == nil {
				if result.Flow != nil || result.Reason != tc.Reason {
					t.Fatalf("got %+v, want unavailable (%s)", result, tc.Reason)
				}
				return
			}
			if result.Flow == nil || math.Abs(*result.Flow-*tc.Flow) > 1e-9 {
				t.Fatalf("got %+v, want flow %.12f", result, *tc.Flow)
			}
		})
	}
	c, _ := computer(t, now)
	for _, row := range fixture.Trip {
		at := now.Add(time.Duration(row.MS) * time.Millisecond)
		c.Observe(at, row.snapshot(at))
	}
	s := c.state
	actual := map[string]float64{
		"liters": s.BaseLiters, "joint_liters": s.JointLiters, "km": s.DistanceKM, "covered_km": s.CoveredKM,
		"observed_s": float64(s.ObservedNS) / 1e9, "covered_s": float64(s.CoveredNS) / 1e9, "gap_s": float64(s.GapNS) / 1e9,
	}
	for key, want := range fixture.Totals {
		if got, ok := actual[key]; !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("%s got %.12f, want %.12f", key, got, want)
		}
	}
	if !s.Incomplete {
		t.Error("expected incomplete coverage")
	}
}
