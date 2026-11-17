package milanalysis

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

const testSession = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// All envelopes, timing and signal scenarios in this file are synthetic.
var epoch = time.Date(2026, 11, 7, 13, 0, 0, 0, time.UTC)

func row(t *testing.T, kind, incident string, at time.Time, data any) milstore.Entry {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return milstore.Entry{Kind: kind, Incident: incident, At: at, Data: b}
}

func archive(t *testing.T, entries []milstore.Entry) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "mil-part-0000000000000001.jsonl"
	all := append([]milstore.Entry{{Kind: "session_start", At: epoch}}, entries...)
	all = append(all, milstore.Entry{Kind: "session_end", At: epoch.Add(10 * time.Minute), Data: json.RawMessage(`{"reason":"closed"}`)})
	var body []byte
	for i, e := range all {
		b, err := json.Marshal(struct {
			Version   int    `json:"version"`
			Session   string `json:"session_id"`
			Source    string `json:"source"`
			Sequence  uint64 `json:"sequence"`
			ElapsedNS int64  `json:"elapsed_ns"`
			milstore.Entry
		}{1, testSession, "live", uint64(i + 1), int64(i), e})
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, b...)
		body = append(body, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(map[string]any{"version": 1, "source": "live", "next": 1, "retention_truncated": false, "parts": []any{map[string]any{"name": name, "session_id": testSession, "started_at": epoch, "closed": true}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mil-manifest.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sample(at time.Time, values map[string]float64) mil.Frame {
	s := telemetry.Snapshot{Source: "live", Health: telemetry.Connected, Generation: 1, CapturedAt: at, ReceivedAt: at, Signals: make(map[string]telemetry.Signal)}
	units := map[string]string{"rpm": "rpm", "speed_kmh": "km/h", "map_kpa": "kPa", "iat_c": "degC", "coolant_c": "degC", "load_percent": "%", "throttle_percent": "%", "stft_b1_percent": "%", "ltft_b1_percent": "%", "o2_sensor1_v": "V", "o2_sensor2_v": "V", "voltage_v": "V"}
	for key, v := range values {
		s.Signals[key] = telemetry.Signal{Value: new(v), Unit: units[key], Source: "ecu_mode01", Support: "supported", Status: "available", CapturedAt: at, ReceivedAt: at, StaleAfterMS: 20000}
	}
	if v, ok := s.Signals["voltage_v"]; ok {
		v.Source = "adapter"
		s.Signals["voltage_v"] = v
	}
	return mil.NewFrame(at, s, 3*time.Second)
}

func incidentRows(t *testing.T, frames []mil.Frame) []milstore.Entry {
	t.Helper()
	w := mil.Window{ID: "incident-0000000000000001", TriggeredAt: epoch.Add(time.Minute), RequestedPreFrom: epoch, RetainedFrom: epoch, PostUntil: epoch.Add(90 * time.Second), EndedAt: epoch.Add(90 * time.Second), PreWindowRetained: true, PostWindowRetained: true, Reason: "window_elapsed"}
	r := []milstore.Entry{row(t, "incident_start", w.ID, w.TriggeredAt, w)}
	for _, f := range frames {
		r = append(r, row(t, "pre", w.ID, f.At, f))
	}
	return append(r, row(t, "incident_end", w.ID, w.EndedAt, w))
}

func codes(r Report) []string {
	var result []string
	for _, s := range r.Sessions {
		for _, f := range s.Findings {
			result = append(result, f.Code)
		}
		for _, i := range s.Incidents {
			for _, f := range i.Findings {
				result = append(result, f.Code)
			}
		}
	}
	return result
}

func TestAnalysisSeparatesStoredEvidenceAndLateDiagnostics(t *testing.T) {
	t.Parallel()
	frames := []mil.Frame{sample(epoch, map[string]float64{"rpm": 900, "iat_c": 13, "o2_sensor1_v": .8})}
	entries := incidentRows(t, frames)
	e := mil.Event{ID: 1, Kind: "MIL_ON", State: "ON", At: epoch.Add(time.Minute), Generation: 1, PreviousCapturedAt: epoch.Add(55 * time.Second), Observation: &telemetry.MILReading{On: new(true), DTCCount: new(1), Status: "available", CapturedAt: epoch.Add(time.Minute), ReceivedAt: epoch.Add(time.Minute), StaleAfterMS: 15000}}
	entries = append(entries, row(t, "event", entries[0].Incident, e.At, e))
	// Actual observed raw packets; envelope/timestamps here remain synthetic.
	for _, d := range []struct{ cmd, status, raw, detail string }{
		{"03", "available", "43 04 20 00 00 00 00 \r\r>", `{"codes":["P0420"],"source":"ecu_mode03","status":"available"}`},
		{"0A", "unsupported", "7F 0A 11 \r\r>", `{"status":"unsupported","source":"ecu_mode0a"}`},
		{"020200", "available", "42 02 00 04 20 \r\r>", `{"codes":["P0420"],"frame":0,"pid":"02","status":"available","source":"ecu_mode02"}`},
	} {
		entries = append(entries, row(t, "diagnostic", "", epoch.Add(100*time.Second), map[string]any{"command": d.cmd, "poll": telemetry.Poll{Command: d.cmd, Status: d.status, At: epoch.Add(100 * time.Second), Generation: 1}, "reading": telemetry.ECUReading{Status: d.status, RawResponse: d.raw, CapturedAt: epoch.Add(100 * time.Second), ReceivedAt: epoch.Add(100 * time.Second), Detail: json.RawMessage(d.detail)}}))
	}
	dir := archive(t, entries)
	r, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions) != 1 || len(r.Sessions[0].Diagnostics) != 3 || len(r.Sessions[0].Events) != 1 {
		t.Fatalf("report=%+v", r)
	}
	s := r.Sessions[0]
	if s.Diagnostics[0].Incident != "" || s.Diagnostics[1].Reading.Status != "unsupported" || s.Events[0].Event.PreviousCapturedAt != e.PreviousCapturedAt {
		t.Fatal("lost evidence boundaries", s)
	}
	if !slices.Contains(codes(r), "p0420") {
		t.Fatal("missing P0420 finding", codes(r))
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := json.Marshal(r2)
	if string(b) != string(b2) {
		t.Fatal("nondeterministic report")
	}
	if !strings.Contains(string(b), "insufficient_data") {
		t.Fatal("sparse context must stay explicit")
	}
}

func TestFreshnessAndCaptureDedupProtectRules(t *testing.T) {
	t.Parallel()
	f := sample(epoch, map[string]float64{"rpm": 900, "iat_c": -40, "coolant_c": 85})
	duplicate := f
	duplicate.At = epoch.Add(time.Second)
	stale := sample(epoch.Add(2*time.Second), map[string]float64{"rpm": 900, "iat_c": -40})
	i := stale.Snapshot.Signals["iat_c"]
	i.CapturedAt = epoch.Add(-time.Minute)
	stale.Snapshot.Signals["iat_c"] = i
	future := sample(epoch.Add(3*time.Second), map[string]float64{"rpm": 900, "iat_c": 120})
	i = future.Snapshot.Signals["iat_c"]
	i.CapturedAt = future.At.Add(time.Second)
	future.Snapshot.Signals["iat_c"] = i
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f, duplicate, stale, future})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range r.Sessions[0].Incidents[0].Findings {
		if v.Code == "iat_out_of_range" && v.Count != 1 {
			t.Fatalf("counted stale/future/duplicate: %+v", v)
		}
	}
	if !slices.Contains(codes(r), "iat_out_of_range") || !slices.Contains(codes(r), "signal_unavailable") {
		t.Fatal(codes(r))
	}
}

func TestSessionAndIncidentSelectionErrors(t *testing.T) {
	t.Parallel()
	dir := archive(t, nil)
	for _, o := range []Options{{Session: "../bad"}, {Incident: "incident-0000000000000001"}, {Session: strings.Repeat("b", 32)}, {Session: testSession, Incident: "incident-0000000000000001"}} {
		if _, err := Analyze(t.Context(), dir, o); err == nil {
			t.Fatal("accepted", o)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Analyze(ctx, dir, Options{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestNoContextDoesNotBecomeHealthyDiagnosis(t *testing.T) {
	t.Parallel()
	e := mil.Event{ID: 1, Kind: "MIL_ALREADY_ON", State: "ON", At: epoch, Generation: 1, Observation: &telemetry.MILReading{On: new(true), DTCCount: new(0), Status: "available", CapturedAt: epoch, ReceivedAt: epoch, StaleAfterMS: 15000}}
	r, err := Analyze(t.Context(), archive(t, []milstore.Entry{row(t, "event", "", epoch, e)}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Limitations) == 0 || r.Sessions[0].Events[0].Event.Kind != "MIL_ALREADY_ON" {
		t.Fatal(r)
	}
	if !strings.Contains(r.Sessions[0].Summary, "Причина не определена") {
		t.Fatal(r.Sessions[0].Summary)
	}
}
