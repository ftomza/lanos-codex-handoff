package fuel

import (
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type Result struct {
	Terms                                  *telemetry.SpeedDensityTerms
	Flow                                   *float64
	Until                                  time.Time
	Method, Provenance, Confidence, Reason string
}

type Estimator interface {
	Estimate(Config, telemetry.Snapshot, time.Time) Result
}
type directEstimator struct{}
type mafEstimator struct{}
type densityEstimator struct{}

func Select(c Config, s telemetry.Snapshot) string {
	if c.Model != "auto" {
		return c.Model
	}
	for _, m := range []struct{ method, key string }{{"direct", "fuel_rate_lph"}, {"maf", "maf_g_s"}, {"speed_density", "map_kpa"}} {
		if s.Signals[m.key].Support == "supported" {
			return m.method
		}
	}
	return ""
}

func Evaluate(c Config, s telemetry.Snapshot, method string, now time.Time) Result {
	if err := c.Validate(); err != nil {
		return Result{Method: method, Reason: "invalid_config"}
	}
	s = s.At(now, c.StaleAfter)
	if s.Health != telemetry.Connected {
		return Result{Method: method, Reason: "source_unavailable"}
	}
	var estimator Estimator
	switch method {
	case "direct":
		estimator = directEstimator{}
	case "maf":
		estimator = mafEstimator{}
	case "speed_density":
		estimator = densityEstimator{}
	default:
		return Result{Reason: "no_supported_model"}
	}
	r := estimator.Estimate(c, s, now)
	r.Method = method
	if r.Flow != nil && !bounded(*r.Flow, 0, c.MaxFlow) {
		r.Flow = nil
		r.Reason = "flow_out_of_range"
	}
	return r
}

// signal validates individual and global timestamps through Snapshot.At first.
func signal(s telemetry.Snapshot, key string, lo, hi float64) (*float64, time.Time) {
	v := s.Signals[key]
	if v.Status != "available" || v.Value == nil || !bounded(*v.Value, lo, hi) {
		return nil, time.Time{}
	}
	until := minTime(v.CapturedAt, v.ReceivedAt).Add(time.Duration(v.StaleAfterMS) * time.Millisecond)
	return v.Value, until
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func horizon(c Config, s telemetry.Snapshot, until time.Time) time.Time {
	return minTime(until, minTime(s.CapturedAt, s.ReceivedAt).Add(c.StaleAfter))
}
func unavailable(reason string) Result { return Result{Reason: reason, Confidence: "unknown"} }

func (directEstimator) Estimate(c Config, s telemetry.Snapshot, _ time.Time) Result {
	v, until := signal(s, "fuel_rate_lph", 0, c.MaxFlow)
	if v == nil {
		return unavailable("fuel_rate_unavailable")
	}
	return Result{Flow: new(*v), Until: horizon(c, s, until), Provenance: "ecu_reported", Confidence: "unverified"}
}
func (mafEstimator) Estimate(c Config, s telemetry.Snapshot, _ time.Time) Result {
	v, until := signal(s, "maf_g_s", 0, c.MaxMAF)
	if v == nil {
		return unavailable("maf_unavailable")
	}
	return Result{Flow: new(*v * 3600 / (c.AFR * c.Density)), Until: horizon(c, s, until), Provenance: "estimated_maf", Confidence: "low"}
}
func (densityEstimator) Estimate(c Config, s telemetry.Snapshot, _ time.Time) Result {
	rpm, until := signal(s, "rpm", 0, 7000)
	if rpm == nil {
		return unavailable("rpm_unavailable")
	}
	if *rpm == 0 {
		return Result{Terms: &telemetry.SpeedDensityTerms{EngineStopped: true, RawFlowLPH: new(0.0), VE: c.VE, AFR: c.AFR, FuelDensityGL: c.Density}, Flow: new(0.0), Until: horizon(c, s, until), Provenance: "estimated_speed_density", Confidence: "low"}
	}
	if s.Source == telemetry.Live && !c.AllowUnverifiedIAT {
		return unavailable("unverified_iat")
	}
	pressure, pUntil := signal(s, "map_kpa", .001, 120)
	temp, tUntil := signal(s, "iat_c", -29.999, 100)
	if pressure == nil {
		return unavailable("map_unavailable")
	}
	if temp == nil {
		return unavailable("iat_unavailable_or_out_of_range")
	}
	rho := *pressure * 1000 / (287.05 * (*temp + 273.15))
	air := rho * (c.Displacement / 1000) * (*rpm / 120) * c.VE * 1000
	return Result{Terms: &telemetry.SpeedDensityTerms{PressurePa: new(*pressure * 1000), TemperatureK: new(*temp + 273.15), DensityKGM3: new(rho), VolumeM3S: new(c.Displacement / 1000 * *rpm / 120 * c.VE), AirGS: new(air), RawFlowLPH: new(air * 3600 / (c.AFR * c.Density)), VE: c.VE, AFR: c.AFR, FuelDensityGL: c.Density}, Flow: new(air * 3600 / (c.AFR * c.Density)), Until: horizon(c, s, minTime(until, minTime(pUntil, tUntil))), Provenance: "estimated_speed_density", Confidence: "low"}
}
