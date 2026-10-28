package fuelhistory

import (
	"math"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func fresh(e Entry, key string) *float64 {
	if e.Sample == nil {
		return nil
	}
	s := e.effective
	if s == nil {
		copy := e.Sample.Snapshot.At(e.At, 3*time.Second)
		s = &copy
	}
	v := s.Signals[key]
	if v.Status != "available" || v.Value == nil || !finite(*v.Value) {
		return nil
	}
	bounds := map[string][2]float64{"rpm": {0, 7000}, "speed_kmh": {0, 220}, "map_kpa": {.001, 120}}
	if bound, ok := bounds[key]; ok && (*v.Value < bound[0] || *v.Value > bound[1]) {
		return nil
	}
	return v.Value
}
func comparable(a, b Entry) bool {
	x, y := a.Sample, b.Sample
	return x != nil && y != nil && !b.Boundary && a.Session == b.Session && x.Snapshot.Generation == y.Snapshot.Generation && x.Fingerprint == y.Fingerprint && x.Fuel.Method == y.Fuel.Method && x.Fuel.Revision == y.Fuel.Revision && x.Fuel.ResetAt.Equal(y.Fuel.ResetAt) && x.Fuel.Factor == y.Fuel.Factor
}
func interval(a, b Entry, dt float64) (Stats, bool) {
	if !comparable(a, b) || dt <= 0 || dt > 3 {
		return Stats{}, false
	}
	x, y := a.Sample, b.Sample
	v := Stats{Seconds: dt, Liters: (y.BaseLiters - x.BaseLiters) * y.Fuel.Factor, DistanceKM: y.Fuel.DistanceKM - x.Fuel.DistanceKM, JointLiters: (y.JointLiters - x.JointLiters) * y.Fuel.Factor, CoveredKM: y.Fuel.CoveredKM - x.Fuel.CoveredKM, CoveredSeconds: y.Fuel.CoveredSeconds - x.Fuel.CoveredSeconds}
	for _, n := range []float64{v.Liters, v.DistanceKM, v.JointLiters, v.CoveredKM, v.CoveredSeconds} {
		if !finite(n) || n < 0 {
			return Stats{}, false
		}
	}
	// Deliberately conservative physical/model ceilings; corrupt counters cannot
	// create arbitrary consumption. Partial covered intervals stay unclassified.
	if v.Liters > 200*4*dt/3600+1e-8 || v.DistanceKM > 220*dt/3600+1e-8 || v.CoveredSeconds > dt+1e-6 || v.JointLiters > v.Liters+1e-8 || v.CoveredKM > v.DistanceKM+1e-8 {
		return Stats{}, false
	}
	if fresh(a, "speed_kmh") != nil && fresh(b, "speed_kmh") != nil {
		sig := a.Sample.Snapshot.Signals["speed_kmh"]
		until := sig.CapturedAt.Add(time.Duration(sig.StaleAfterMS) * time.Millisecond)
		if b.At.Before(until) {
			v.SpeedSeconds = dt
			v.SpeedDistanceKM = v.DistanceKM
		}
	}
	return v, true
}
func flow(e Entry) *float64 {
	if e.Sample == nil {
		return nil
	}
	f := e.effectiveFuel
	if f == nil {
		s := e.Sample.Snapshot.At(e.At, 3*time.Second)
		copy := e.Sample.Fuel.At(e.At, s.Health)
		f = &copy
	}
	if f.FlowLPH == nil || !finite(*f.FlowLPH) || *f.FlowLPH < 0 || *f.FlowLPH > 800 {
		return nil
	}
	return f.FlowLPH
}
func mode(e Entry) string {
	speed := fresh(e, "speed_kmh")
	rpm := fresh(e, "rpm")
	if speed == nil {
		return "unknown"
	}
	if *speed > 0 {
		return "moving"
	}
	if *speed != 0 || rpm == nil {
		return "unknown"
	}
	if *rpm > 0 {
		return "idle"
	}
	if *rpm == 0 {
		return "engine_off"
	}
	return "unknown"
}

var ranges = map[string][]float64{"rpm": {0, 1000, 2000, 3000}, "speed_kmh": {0, 5, 30, 60}, "map_kpa": {30, 60, 90}}

func bucket(key string, e Entry) string {
	value := fresh(e, key)
	if value == nil {
		return "unknown"
	}
	v := *value
	if v < 0 || key == "rpm" && v > 7000 || key == "speed_kmh" && v > 220 || key == "map_kpa" && (v <= 0 || v > 120) {
		return "unknown"
	}
	if (key == "rpm" || key == "speed_kmh") && v == 0 {
		return "0"
	}
	cuts := ranges[key]
	labels := map[string][]string{"rpm": {"1-999", "1000-1999", "2000-2999", "3000+"}, "speed_kmh": {"(0,5)", "[5,30)", "[30,60)", "60+"}, "map_kpa": {"(0,30)", "[30,60)", "[60,90)", "90+"}}
	offset := 0
	if key != "map_kpa" {
		offset = 1
	}
	for i := offset; i < len(cuts); i++ {
		if v < cuts[i] {
			return labels[key][i-offset]
		}
	}
	return labels[key][len(labels[key])-1]
}
func pairBucket(key string, a, b Entry) string {
	x, y := bucket(key, a), bucket(key, b)
	if x != y {
		return "mixed"
	}
	return x
}
func knownDuration(v float64) bool {
	return finite(v) && v > 0
}
func health(e Entry) telemetry.Health {
	if e.Sample == nil {
		return telemetry.Disconnected
	}
	if e.effective != nil {
		return e.effective.Health
	}
	return e.Sample.Snapshot.At(e.At, 3*time.Second).Health
}
