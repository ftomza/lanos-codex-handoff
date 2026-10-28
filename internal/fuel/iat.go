package fuel

import (
	"math"
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type iatTracker struct {
	cold                 bool
	generation           uint64
	last, since, running time.Time
	value, low, high     float64
	count                int
	flags                []string
}

func (t *iatTracker) flag(code string) {
	if !slices.Contains(t.flags, code) {
		t.flags = append(t.flags, code)
	}
}
func (t *iatTracker) observe(now time.Time, raw telemetry.Snapshot) {
	if raw.Generation != t.generation {
		*t = iatTracker{cold: t.cold, generation: raw.Generation, running: t.running}
	}
	s := raw
	iat := s.Signals["iat_c"]
	rpm := s.Signals["rpm"]
	running := ownFresh(rpm, now) && *rpm.Value > 0
	if s.Health != telemetry.Connected && s.Health != telemetry.Connecting || !ownFresh(iat, now) {
		t.last, t.since = time.Time{}, time.Time{}
		t.count = 0
		return
	}
	if running && t.running.IsZero() {
		t.running = now
	}
	if t.cold && running && now.Sub(t.running) <= 30*time.Second {
		coolant := s.Signals["coolant_c"]
		if ownFresh(coolant, now) && math.Abs(float64(coolant.CapturedAt.Sub(iat.CapturedAt))) <= float64(5*time.Second) && math.Abs(*coolant.Value-*iat.Value) > 15 {
			t.flag("iat_cold_mismatch")
		}
	}
	if !iat.CapturedAt.After(t.last) {
		return
	}
	value := *iat.Value
	if value <= -30 || value > 100 {
		t.flag("iat_out_of_range")
	}
	dt := iat.CapturedAt.Sub(t.last)
	if !t.last.IsZero() && dt > 0 && dt <= 10*time.Second && math.Abs(value-t.value) >= 15 {
		t.flag("iat_jump")
	}
	if !running || t.last.IsZero() || dt > 20*time.Second || math.Max(t.high, value)-math.Min(t.low, value) > .5 {
		t.since, t.low, t.high, t.count = iat.CapturedAt, value, value, 0
	}
	t.low, t.high = math.Min(t.low, value), math.Max(t.high, value)
	t.count++
	if running && t.count >= 10 && iat.CapturedAt.Sub(t.since) >= 5*time.Minute {
		t.flag("iat_constant")
	}
	t.last, t.value = iat.CapturedAt, value
}

func (c *Computer) quality(f *telemetry.Fuel) {
	f.QualityReasons = slices.Clone(c.iat.flags)
	f.Quality = "VALID"
	if f.Method == "speed_density" {
		f.QualityReasons = append(f.QualityReasons, "assumed_ve", "nominal_fuel_properties")
		if c.state.Source == telemetry.Live {
			f.QualityReasons = append(f.QualityReasons, "unverified_iat")
		}
	}
	if f.Method == "maf" {
		f.QualityReasons = append(f.QualityReasons, "nominal_fuel_properties")
	}
	if len(f.QualityReasons) > 0 {
		f.Quality = "DEGRADED"
	}
	if f.FlowLPH == nil || f.Status != "available" {
		f.Quality = "UNAVAILABLE"
	}
	if !c.iat.cold {
		f.ColdStartCheck = "not_evaluated"
	} else {
		f.ColdStartCheck = "owner_annotated"
	}
}

func ownFresh(s telemetry.Signal, now time.Time) bool {
	if s.Status != "available" || s.Value == nil || !finite(*s.Value) || s.StaleAfterMS <= 0 {
		return false
	}
	for _, at := range []time.Time{s.CapturedAt, s.ReceivedAt} {
		age := now.Sub(at)
		if at.IsZero() || age < 0 || age >= time.Duration(s.StaleAfterMS)*time.Millisecond {
			return false
		}
	}
	return true
}
