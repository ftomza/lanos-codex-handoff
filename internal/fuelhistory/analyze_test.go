package fuelhistory

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func syntheticHistory(t *testing.T, rows []Entry) string {
	t.Helper()
	dir := t.TempDir()
	session := "0123456789abcdef0123456789abcdef"
	m := manifest{Version: 1, Source: telemetry.Simulator, Next: 1, Parts: []part{{Name: "part-0000000000000001.jsonl", Session: session, StartedAt: rows[0].At, Closed: true}}}
	f, err := os.OpenFile(filepath.Join(dir, m.Parts[0].Name), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range rows {
		e.Version = 1
		e.Source = telemetry.Simulator
		e.Session = session
		e.Sequence = uint64(i + 1)
		if err := json.NewEncoder(f).Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	return dir
}
func sample(at time.Time, seconds int64, liters, km float64) Entry {
	d := diagnostic(at)
	d.BaseLiters = liters
	d.JointLiters = liters
	d.Fingerprint = "model"
	d.Fuel.Method = "direct"
	d.Fuel.Status = "available"
	d.Fuel.FlowLPH = new(3.6)
	d.Fuel.FlowUntil = at.Add(3 * time.Second)
	d.Fuel.SpeedUntil = at.Add(3 * time.Second)
	d.Fuel.Speed = new(36.0)
	d.Fuel.Liters = liters
	d.Fuel.DistanceKM = km
	d.Fuel.CoveredKM = km
	d.Fuel.CoveredSeconds = float64(seconds)
	d.Fuel.ObservedSeconds = float64(seconds)
	d.Fuel.ResetAt = time.Unix(0, 0)
	d.Snapshot.CapturedAt = at
	d.Snapshot.ReceivedAt = at
	d.Snapshot.Signals = map[string]telemetry.Signal{}
	for k, v := range map[string]float64{"rpm": 1500, "speed_kmh": 36, "map_kpa": 40, "iat_c": 20} {
		d.Snapshot.Signals[k] = telemetry.Signal{Value: new(v), Status: "available", CapturedAt: at, ReceivedAt: at, StaleAfterMS: 3000}
	}
	return Entry{Kind: "sample", At: at, ElapsedNS: seconds * int64(time.Second), Sample: &d}
}
func TestAnalyzeUsesCountersAndNeverBridgesResetOrDisconnect(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	a, b := sample(now, 0, 0, 0), sample(now.Add(time.Second), 1, .001, .01)
	c := sample(now.Add(2*time.Second), 2, .001, .01)
	c.Sample.Snapshot.Generation = 2
	d := sample(now.Add(3*time.Second), 3, .002, .02)
	d.Sample.Snapshot.Generation = 2
	end := Entry{Kind: "session_end", At: now.Add(3 * time.Second), ElapsedNS: int64(3 * time.Second), Reason: "closed"}
	dir := syntheticHistory(t, []Entry{{Kind: "session_start", At: now}, a, b, c, d, end})
	report, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(report.Liters-.002) > 1e-12 || math.Abs(report.DistanceKM-.02) > 1e-12 || report.DurationSeconds != 3 || !report.Incomplete || report.GapSeconds < 1 {
		t.Fatalf("wrong accounting: %+v", report)
	}
	if report.AverageL100 != nil || report.Modes["moving"].Seconds != 2 {
		t.Fatal("wrong weighted report", report)
	}
	if len(report.Series["rpm"]) != 4 {
		t.Fatal("no saved series")
	}
}
func TestAnalysisDoesNotTreatCalibrationJumpAsFuel(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	a := sample(now, 0, 1, 10)
	b := sample(now.Add(time.Second), 1, 1.001, 10.01)
	b.Sample.Fuel.Factor = 2
	b.Sample.Fuel.Liters = 2.002
	b.Sample.Fuel.Revision = 2
	dir := syntheticHistory(t, []Entry{{Kind: "session_start", At: now}, a, b, {Kind: "session_end", At: b.At, Reason: "closed"}})
	report, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Liters != 0 || !report.Incomplete {
		t.Fatal("calibration became consumption", report)
	}
}
func TestSeriesKeepsExtremaAndGapsWithinBound(t *testing.T) {
	t.Parallel()
	c := newSeries(40)
	now := time.Now()
	for i := range 1000 {
		v := float64(i % 20)
		if i == 500 {
			v = 999
		}
		p := Point{At: now.Add(time.Duration(i) * time.Second), Value: new(v)}
		if i == 550 {
			p.Value = nil
			p.Break = true
		}
		c.add(p)
	}
	points := c.finish()
	peak, gap := false, false
	for _, p := range points {
		peak = peak || p.Value != nil && *p.Value == 999
		gap = gap || p.Break
	}
	if len(points) > 40 || !peak || !gap {
		t.Fatalf("bad reduction len=%d peak=%t gap=%t", len(points), peak, gap)
	}
}

func TestTinyChartBudgetsMakeProgressWithoutBridgingGaps(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()
			c := newSeries(limit)
			for i := range 3 {
				c.add(Point{At: time.Unix(int64(i), 0), Value: new(float64(i)), Break: i == 1})
			}
			c.reduce()
			if len(c.points) > limit {
				t.Fatal("reduction cannot terminate at requested limit", len(c.points))
			}
			for _, p := range c.finish() {
				if !p.Break {
					t.Fatal("tiny budget bridges discarded history")
				}
			}
		})
	}
}

func TestLongGapBreaksEverySavedChart(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	a := sample(now, 0, 0, 0)
	b := sample(now.Add(10*time.Second), 10, .001, .01)
	dir := syntheticHistory(t, []Entry{{Kind: "session_start", At: now}, a, b, {Kind: "session_end", At: b.At, Reason: "closed"}})
	r, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"rpm", "speed_kmh", "map_kpa", "flow_lph", "liters", "average_l100"} {
		if !r.Series[key][1].Break {
			t.Fatalf("%s bridges a ten-second gap", key)
		}
	}
	if r.Liters != 0 || r.GapSeconds != 10 || r.Modes["unknown"].Seconds != 10 {
		t.Fatal("long gap counted", r)
	}
}

func TestSelectedRangeKeepsLifecycleWithoutFalseSequenceGaps(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	rows := []Entry{{Kind: "session_start", At: now}}
	for i := range 5 {
		rows = append(rows, sample(now.Add(time.Duration(i)*time.Second), int64(i), float64(i)*.001, float64(i)*.01))
	}
	rows = append(rows, Entry{Kind: "session_end", At: now.Add(5 * time.Second), Reason: "closed"})
	dir := syntheticHistory(t, rows)
	r, err := Analyze(t.Context(), dir, Options{From: now.Add(time.Second), To: now.Add(2 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if r.Incomplete || r.DurationSeconds != 1 || math.Abs(r.Liters-.001) > 1e-12 {
		t.Fatalf("filtered range diagnosed as corrupt: %+v", r)
	}
}
