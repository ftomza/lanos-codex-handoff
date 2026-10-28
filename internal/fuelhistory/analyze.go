package fuelhistory

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type analysis struct {
	selected       bool
	report         Report
	previous       *Entry
	sequences      map[string]uint64
	starts, ends   map[string]bool
	lastFlags      map[string]bool
	charts         map[string]*series
	minute         int64
	segmentSession string
}

func Analyze(ctx context.Context, dir string, o Options) (Report, error) {
	if err := o.Validate(); err != nil {
		return Report{}, err
	}
	m, err := loadManifest(dir)
	if err != nil {
		return Report{}, err
	}
	limit := o.MaxPoints
	if limit == 0 {
		limit = 4000
	}
	a := &analysis{selected: !o.From.IsZero() || !o.To.IsZero(), report: Report{Version: Version, Source: m.Source, RetentionTruncated: m.Truncated, Issues: []Issue{}, Anomalies: []Anomaly{}, Markers: []Marker{}, Segments: []Segment{}, Modes: make(map[string]*Stats, 4), Distributions: make(map[string][]Bin, 3), Series: make(map[string][]Point, 6), CoverageScope: "saved observed intervals only; gaps between processes are unknown"}, sequences: make(map[string]uint64, 32), starts: make(map[string]bool, 32), ends: make(map[string]bool, 32), lastFlags: make(map[string]bool, 16), charts: make(map[string]*series, 6), minute: -1}
	for _, key := range []string{"rpm", "speed_kmh", "map_kpa", "flow_lph", "liters", "average_l100"} {
		a.charts[key] = newSeries(limit)
	}
	for _, key := range []string{"moving", "idle", "engine_off", "unknown"} {
		a.report.Modes[key] = &Stats{}
	}
	issues, err := Read(ctx, dir, o, a.consume)
	if err != nil {
		return Report{}, err
	}
	a.report.Issues = append(a.report.Issues, issues...)
	for session := range a.sequences {
		if !a.starts[session] || !a.ends[session] {
			a.issue("incomplete_session", session)
		}
	}
	if a.report.Rows == 0 {
		return a.report, fmt.Errorf("no history rows match selection")
	}
	a.finish()
	return a.report, nil
}
func (a *analysis) issue(code, detail string) {
	a.report.Incomplete = true
	if len(a.report.Issues) < 128 {
		a.report.Issues = append(a.report.Issues, Issue{Code: code, Detail: detail})
	}
}
func (a *analysis) marker(e Entry, reason string) {
	if len(a.report.Markers) < 256 {
		a.report.Markers = append(a.report.Markers, Marker{At: e.At, Reason: reason})
	}
}
func (a *analysis) consume(e Entry) error {
	if e.Sample != nil {
		effective := e.Sample.Snapshot.At(e.At, 3*time.Second)
		effectiveFuel := e.Sample.Fuel.At(e.At, effective.Health)
		e.effective, e.effectiveFuel = &effective, &effectiveFuel
	}
	a.report.Rows++
	last := a.sequences[e.Session]
	if last == 0 && e.Sequence != 1 || last > 0 && e.Sequence != last+1 {
		filteredBoundary := e.Boundary
		e.Boundary = true
		if !a.selected || !filteredBoundary {
			a.issue("sequence_gap", e.Session)
		}
	}
	a.sequences[e.Session] = e.Sequence
	if e.Kind == "session_start" {
		a.starts[e.Session] = true
		a.previous = nil
		return nil
	}
	if e.Kind == "session_end" {
		a.ends[e.Session] = e.Reason == "closed"
		if e.Reason == "recording_failed" {
			a.issue("recording_failed", e.Error)
		}
		a.previous = nil
		return nil
	}
	if e.Sample == nil {
		return nil
	}
	if a.report.From.IsZero() {
		a.report.From = e.At
	}
	if e.Kind == "sample" {
		a.report.Samples++
	}
	a.report.To = e.At
	if e.Kind == "event" {
		a.marker(e, e.Reason)
	}
	boundary := e.Boundary || e.Kind == "event" && e.Reason != "availability_changed" && e.Reason != "computer_close"
	if a.previous != nil && a.previous.Session == e.Session {
		dt := float64(e.ElapsedNS-a.previous.ElapsedNS) / 1e9
		if knownDuration(dt) {
			a.report.DurationSeconds += dt
			v, ok := interval(*a.previous, e, dt)
			if e.At.Before(a.previous.At) {
				ok = false
				a.issue("clock_reversal", "UTC time moved backwards")
			}
			if boundary {
				ok = false
			}
			if ok {
				a.addInterval(*a.previous, e, v)
			} else {
				boundary = true
				a.report.GapSeconds += dt
				a.report.Modes["unknown"].Seconds += dt
				a.segment(e, Stats{Seconds: dt})
				a.report.Incomplete = true
				a.marker(e, "gap")
			}
		} else if dt < 0 {
			boundary = true
			a.issue("elapsed_reversal", "elapsed time moved backwards")
		}
	}
	a.points(e, boundary)
	a.anomalies(e)
	a.previous = &e
	a.report.Last = &e
	return nil
}
func (a *analysis) addInterval(prev, e Entry, v Stats) {
	a.report.Stats.add(v)
	a.report.GapSeconds += v.Seconds - v.CoveredSeconds
	if v.CoveredSeconds < v.Seconds-1e-6 {
		a.report.Incomplete = true
	}
	category := mode(e)
	if mode(prev) != category || v.CoveredSeconds < v.Seconds-1e-6 {
		category = "unknown"
	}
	a.report.Modes[category].add(v)
	for _, key := range []string{"rpm", "speed_kmh", "map_kpa"} {
		label := pairBucket(key, prev, e)
		if v.CoveredSeconds < v.Seconds-1e-6 {
			label = "unknown"
		}
		bins := a.report.Distributions[key]
		i := slices.IndexFunc(bins, func(b Bin) bool { return b.Label == label })
		if i < 0 {
			bins = append(bins, Bin{Label: label})
			i = len(bins) - 1
		}
		bins[i].Stats.add(v)
		a.report.Distributions[key] = bins
	}
	x, y := flow(prev), flow(e)
	if x != nil && y != nil && v.CoveredSeconds >= v.Seconds-1e-6 && e.At.Before(prev.Sample.Fuel.FlowUntil) && health(prev) == telemetry.Connected && health(e) == telemetry.Connected {
		a.report.SampledLiters += (*x + *y) / 2 * v.Seconds / 3600
	}
	a.segment(e, v)
}
func (a *analysis) segment(e Entry, v Stats) {
	minute := e.ElapsedNS / int64(time.Minute)
	reason := fmt.Sprintf("%s/%d/%s/%d/%g", e.Session, e.Sample.Snapshot.Generation, e.Sample.Fuel.Method, e.Sample.Fuel.Revision, e.Sample.Fuel.Factor)
	if len(a.report.Segments) == 0 || a.minute != minute || a.segmentSession != reason {
		if len(a.report.Segments) >= 4096 {
			a.issue("segment_limit", "segment detail truncated")
			return
		}
		a.report.Segments = append(a.report.Segments, Segment{Session: e.Session, From: e.At.Add(-time.Duration(v.Seconds * 1e9)), Reason: reason})
		a.minute, a.segmentSession = minute, reason
	}
	s := &a.report.Segments[len(a.report.Segments)-1]
	s.To = e.At
	s.Stats.add(v)
}
func (a *analysis) points(e Entry, boundary bool) {
	stopped := health(e) != telemetry.Connected
	if a.previous != nil && a.previous.Sample != nil && a.previous.Sample.Snapshot.Generation != e.Sample.Snapshot.Generation {
		boundary = true
	}
	for _, key := range []string{"rpm", "speed_kmh", "map_kpa"} {
		a.charts[key].add(Point{At: e.At, Value: fresh(e, key), Break: boundary || stopped})
	}
	a.charts["flow_lph"].add(Point{At: e.At, Value: flow(e), Break: boundary || stopped})
	a.charts["liters"].add(Point{At: e.At, Value: new(e.Sample.Fuel.Liters), Break: boundary || stopped})
	a.charts["average_l100"].add(Point{At: e.At, Value: e.Sample.Fuel.AverageL100, Break: boundary || stopped})
	speed := fresh(e, "speed_kmh")
	if speed != nil && (a.report.MaxSpeed == nil || *speed > *a.report.MaxSpeed) {
		a.report.MaxSpeed = new(*speed)
	}
}
func (a *analysis) anomalies(e Entry) {
	current := make(map[string]bool, 16)
	for _, code := range e.Sample.Fuel.QualityReasons {
		if !strings.HasPrefix(code, "iat_") {
			continue
		}
		current[code] = true
		if a.lastFlags[code] {
			continue
		}
		a.report.AnomalyCount++
		if len(a.report.Anomalies) < 128 {
			a.report.Anomalies = append(a.report.Anomalies, Anomaly{At: e.At, Code: code})
		}
	}
	a.lastFlags = current
}
func (a *analysis) finish() {
	for key, chart := range a.charts {
		a.report.Series[key] = chart.finish()
		a.report.Downsampled = a.report.Downsampled || chart.reduced
	}
	for _, stats := range a.report.Modes {
		stats.finish()
	}
	for key, bins := range a.report.Distributions {
		for i := range bins {
			bins[i].finish()
		}
		a.report.Distributions[key] = bins
	}
	for i := range a.report.Segments {
		a.report.Segments[i].finish()
	}
	a.report.Stats.finish()
	if len(a.report.Issues) > 0 {
		a.report.Incomplete = true
	}
	if a.report.DurationSeconds > 0 {
		a.report.CoveragePercent = 100 * a.report.CoveredSeconds / a.report.DurationSeconds
	}
}
