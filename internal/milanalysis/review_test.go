package milanalysis

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestFailedRecordingFooterIsNotCleanClosure(t *testing.T) {
	t.Parallel()
	dir := archive(t, nil)
	path := filepath.Join(dir, "mil-part-0000000000000001.jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b = bytes.ReplaceAll(b, []byte(`"reason":"closed"`), []byte(`"reason":"recording_failed","error":"queue full"`))
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Incomplete || !r.Sessions[0].Incomplete || r.Sessions[0].EndReason != "recording_failed" || r.Sessions[0].Error != "queue full" {
		t.Fatal(r)
	}
}

func TestTemporaryUnavailablePollIsReportedOnce(t *testing.T) {
	t.Parallel()
	f := sample(epoch, map[string]float64{"rpm": 900})
	f.Snapshot.Poll = &telemetry.Poll{Command: "010F", Status: "temporarily_unavailable", At: epoch, Generation: 1}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f, f})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(codes(r), "protocol_error") {
		t.Fatal(codes(r))
	}
}

func TestAllRulesExposeInsufficientData(t *testing.T) {
	t.Parallel()
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, nil)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	checks := r.Sessions[0].Incidents[0].Checks
	for _, code := range []string{"iat_out_of_range", "iat_jump", "iat_constant", "map_out_of_range", "map_context_mismatch", "fuel_trim_extreme", "o2_out_of_range", "o2_constant", "coolant_high", "coolant_jump", "adapter_voltage_low", "adapter_voltage_drop"} {
		if !slices.ContainsFunc(checks, func(c RuleCheck) bool { return c.Code == code && c.Status == "insufficient_data" }) {
			t.Fatal("missing check", code, checks)
		}
	}
}

func TestFindingRetainsFullObservationBoundsWithLimitedExamples(t *testing.T) {
	t.Parallel()
	var frames []mil.Frame
	for n := range 20 {
		frames = append(frames, sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900, "iat_c": -40}))
	}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Sessions[0].Incidents[0].Findings {
		if f.Code != "iat_out_of_range" {
			continue
		}
		if f.Count != 20 || len(f.Supporting) > 8 || !f.FirstObservedAt.Equal(epoch) || !f.LastObservedAt.Equal(epoch.Add(19*time.Second)) {
			t.Fatal(f)
		}
		return
	}
	t.Fatal("missing finding")
}

func TestUnrelatedFreshSignalIsNotUnavailable(t *testing.T) {
	t.Parallel()
	f := sample(epoch, map[string]float64{"rpm": 900})
	s := f.Snapshot.Signals["rpm"]
	s.Value = new(0.0)
	s.Unit = "%"
	f.Snapshot.Signals["o2_sensor1_stft_percent"] = s
	f.SignalAges["o2_sensor1_stft_percent"] = mil.SignalAge{Applicable: true}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(codes(r), "signal_unavailable") {
		t.Fatal("unknown-to-rules signal treated as unavailable", codes(r))
	}
}

func TestIncompleteContextEvidenceHasArchiveReference(t *testing.T) {
	t.Parallel()
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, nil)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Sessions[0].Incidents[0].Findings {
		for _, e := range f.Supporting {
			if e.File == "" || e.Line == 0 || e.Sequence == 0 || e.At.IsZero() {
				t.Fatal("untraceable evidence", e)
			}
		}
	}
}

func TestCachedReadingIsNotNewAfterFrameGap(t *testing.T) {
	t.Parallel()
	f := sample(epoch, map[string]float64{"rpm": 900, "iat_c": -40})
	g := f
	g.At = epoch.Add(4 * time.Second)
	g.Snapshot.CapturedAt = g.At
	g.Snapshot.ReceivedAt = g.At
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f, g})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range r.Sessions[0].Incidents[0].Findings {
		if v.Code == "iat_out_of_range" && v.Count != 1 {
			t.Fatal("gap recounted cached reading", v)
		}
	}
}

func TestVaryingDenseOxygenDataIsEvaluated(t *testing.T) {
	t.Parallel()
	var frames []mil.Frame
	for n := range 35 {
		v := .1
		if n%2 == 0 {
			v = .8
		}
		frames = append(frames, sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900, "coolant_c": 85, "o2_sensor1_v": v}))
	}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(codes(r), "o2_constant") {
		t.Fatal(codes(r))
	}
	if !slices.ContainsFunc(r.Sessions[0].Incidents[0].Checks, func(c RuleCheck) bool { return c.Code == "o2_constant" && c.Status == "evaluated" }) {
		t.Fatal("dense O2 was not evaluated")
	}
}

func TestReturningGenerationDoesNotRecountCapture(t *testing.T) {
	t.Parallel()
	a := sample(epoch, map[string]float64{"rpm": 900, "iat_c": -40})
	b := sample(epoch.Add(time.Second), map[string]float64{"rpm": 900, "iat_c": -40})
	b.Snapshot.Generation = 2
	c := a
	c.At = epoch.Add(2 * time.Second)
	c.Snapshot.CapturedAt = c.At
	c.Snapshot.ReceivedAt = c.At
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{a, b, c})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Sessions[0].Incidents[0].Findings {
		if f.Code == "iat_out_of_range" && f.Count != 2 {
			t.Fatal("recounted capture after generation change", f.Count)
		}
	}
}

func TestFrameLimitIsExplicitEvenAfterAnEarlierIntegrityBreak(t *testing.T) {
	t.Parallel()
	frames := make([]mil.Frame, 0, maxFrames+1)
	for n := range maxFrames + 1 {
		frames = append(frames, sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900}))
	}
	entries := incidentRows(t, frames)
	bad := entries[1]
	bad.Data = []byte(`{"at":"2026-11-07T13:00:00Z","snapshot":null}`)
	entries = slices.Insert(entries, 1, bad)
	r, err := Analyze(t.Context(), archive(t, entries), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Sessions[0].Incidents[0].FrameCount != maxFrames || !slices.ContainsFunc(r.Issues, func(i milstore.ReadIssue) bool { return i.Code == "frames_truncated" }) {
		t.Fatal("silent frame truncation", r.Issues)
	}
}

func TestIncidentFilterRetainsOnlySelectedWindow(t *testing.T) {
	t.Parallel()
	entries := incidentRows(t, nil)
	w := mil.Window{ID: "incident-0000000000000002", TriggeredAt: epoch.Add(4 * time.Minute), RequestedPreFrom: epoch.Add(3 * time.Minute), PostUntil: epoch.Add(5 * time.Minute), EndedAt: epoch.Add(5 * time.Minute)}
	entries = append(entries, row(t, "incident_start", w.ID, w.TriggeredAt, w), row(t, "incident_end", w.ID, w.EndedAt, w))
	r, err := Analyze(t.Context(), archive(t, entries), Options{Session: testSession, Incident: w.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions[0].Incidents) != 1 || r.Sessions[0].Incidents[0].ID != w.ID {
		t.Fatal("incorrect incident selection")
	}
}
