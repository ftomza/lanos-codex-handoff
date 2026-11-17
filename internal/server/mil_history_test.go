package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

const syntheticIncident = "incident-0000000000000001"

// Real writer, synthetic payloads: no vehicle or saved private archive needed.
func milHTTPArchive(t *testing.T) (string, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := milstore.Open(milstore.DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	session := w.Status().Session
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	window := mil.Window{ID: syntheticIncident, TriggeredAt: at, RequestedPreFrom: at, RetainedFrom: at, PostUntil: at, EndedAt: at, PreWindowRetained: true, PostWindowRetained: true}
	for _, kind := range []string{"incident_start", "post", "incident_end", "diagnostic"} {
		var v any = window
		incident := syntheticIncident
		if kind == "post" {
			v = map[string]any{"at": at, "snapshot": map[string]any{"source": "live"}, "effective_health": "connected"}
		}
		if kind == "diagnostic" {
			incident = ""
			v = map[string]any{"command": "03", "reading": map[string]any{"status": "available", "captured_at": at, "received_at": at, "detail": map[string]any{"codes": []string{"P0420"}, "source": "ecu_mode03", "status": "available"}}, "poll": map[string]any{"command": "03", "status": "available"}}
		}
		data, _ := json.Marshal(v)
		if err := w.Submit(milstore.Entry{Kind: kind, At: at, Incident: incident, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return dir, session
}

func milHTTPServer(t *testing.T, dir string) *Server {
	t.Helper()
	s, err := New(Config{MILHistoryDir: dir, SourceName: "simulator"})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func milGET(s *Server, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestMILHistoryReportAndFilteredExport(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	before := milHTTPArchiveState(t, dir)
	s := milHTTPServer(t, dir)
	q := "?session=" + session + "&incident=" + syntheticIncident
	w := milGET(s, "/api/mil/history/incident"+q)
	var exported milIncidentExport
	if err := json.Unmarshal(w.Body.Bytes(), &exported); err != nil {
		t.Fatal(err, w.Body.String())
	}
	if w.Code != 200 || !exported.Filtered || exported.Scope != "incident_export" || exported.Source != "live" || exported.Session != session || exported.Incident != syntheticIncident || exported.Incomplete {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(exported.Rows) != 5 {
		t.Fatalf("independent late response exported: %s", w.Body.String())
	}
	for _, row := range exported.Rows {
		if row.File == "" || row.Line <= 0 || row.Kind == "diagnostic" || row.Version != 1 {
			t.Fatalf("bad provenance/envelope: %+v", row)
		}
	}
	for _, path := range []string{"/api/mil/history/sessions", "/api/mil/history/report" + q, "/api/mil/history/incident" + q} {
		w := milGET(s, path)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	w = milGET(s, "/api/mil/history/report"+q)
	var report milanalysis.Report
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil || report.Source != "live" || len(report.Sessions) != 1 || len(report.Sessions[0].Diagnostics) != 1 {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if s.snapshot().Source != "simulator" || s.config.MIL != nil {
		t.Fatal("archive changed current source or enabled capture")
	}
	if !reflect.DeepEqual(before, milHTTPArchiveState(t, dir)) {
		t.Fatal("HTTP changed archive contents, mtimes or listing")
	}
}

func milHTTPArchiveState(t *testing.T, dir string) map[string]struct {
	Hash     [32]byte
	Modified time.Time
} {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := make(map[string]struct {
		Hash     [32]byte
		Modified time.Time
	})
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		state[entry.Name()] = struct {
			Hash     [32]byte
			Modified time.Time
		}{sha256.Sum256(data), info.ModTime()}
	}
	return state
}

func TestMILHistoryRejectsInvalidSelections(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	s := milHTTPServer(t, dir)
	for _, query := range []string{"", "session=", "session=../outside", "session=" + session + "&unknown=x", "session=" + session + "&session=" + session, "session=" + session + "&incident=", "session=" + session + "&incident=bad", "session=" + session + "&bad=%zz", "session=" + session + ";incident=x", "session=" + session + "&" + strings.Repeat("x", 2048)} {
		if w := milGET(s, "/api/mil/history/report?"+query); w.Code != 400 {
			t.Fatalf("query=%q code=%d body=%s", query, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/mil/history/sessions?session=" + session, "/api/mil/history/incident?session=" + session} {
		if w := milGET(s, path); w.Code != 400 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	for _, path := range []string{"/api/mil/history/report?session=" + strings.Repeat("f", 32), "/api/mil/history/incident?session=" + session + "&incident=incident-ffffffffffffffff"} {
		if w := milGET(s, path); w.Code != 409 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestMILHistoryDisabledMissingAndConcurrentGate(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	for _, path := range []string{"sessions", "report?session=" + session, "incident?session=" + session + "&incident=" + syntheticIncident} {
		if w := milGET(milHTTPServer(t, ""), "/api/mil/history/"+path); w.Code != 503 {
			t.Fatal(w.Code, w.Body.String())
		}
		missing := filepath.Join(dir, "missing")
		if w := milGET(milHTTPServer(t, missing), "/api/mil/history/"+path); w.Code != 409 || strings.Contains(w.Body.String(), dir) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	s := milHTTPServer(t, dir)
	s.milHistoryGate <- struct{}{}
	defer func() { <-s.milHistoryGate }()
	for _, path := range []string{"report?session=" + session, "incident?session=" + session + "&incident=" + syntheticIncident} {
		if w := milGET(s, "/api/mil/history/"+path); w.Code != 429 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/snapshot", "/api/mil/history/sessions"} {
		if w := milGET(s, path); w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := milGET(milHTTPServer(t, dir), "/api/mil/history/report?session="+session); w.Code != 200 {
		t.Fatal("gate leaked to another Server", w.Code, w.Body.String())
	}
}

func TestMILHistoryReadersDoNotAcquireTelemetryMutex(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	s := milHTTPServer(t, dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, path := range []string{"report?session=" + session, "incident?session=" + session + "&incident=" + syntheticIncident} {
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() { done <- milGET(s, "/api/mil/history/"+path) }()
		select {
		case w := <-done:
			if w.Code != 200 {
				t.Fatal(w.Code, w.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("history reader waited for telemetry mutex")
		}
	}
}

func TestMILHistoryCancellationAndResponseLimit(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	s := milHTTPServer(t, dir)
	for _, expired := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		if expired {
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		} else {
			cancel()
		}
		for _, path := range []string{"sessions", "report?session=" + session, "incident?session=" + session + "&incident=" + syntheticIncident} {
			r := httptest.NewRequest(http.MethodGet, "/api/mil/history/"+path, nil).WithContext(ctx)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, r)
			if w.Code != 504 {
				t.Fatal(w.Code, w.Body.String())
			}
		}
		cancel()
	}
	w := httptest.NewRecorder()
	writeMILHistory(w, context.Background(), map[string]string{"data": strings.Repeat("x", 8<<20)})
	if w.Code != 413 || strings.HasPrefix(w.Body.String(), "{") {
		t.Fatalf("partial oversized success: %d %s", w.Code, w.Body.String()[:min(w.Body.Len(), 100)])
	}
}

func TestMILHistoryRecoverableDamageAndFailedExport(t *testing.T) {
	t.Parallel()
	for _, damage := range []string{"active", "partial", "corrupt", "retention", "failed", "lost_start", "short_context"} {
		t.Run(damage, func(t *testing.T) {
			dir, session := milHTTPArchive(t)
			manifestPath := filepath.Join(dir, ".mil-manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			var manifest map[string]any
			if err := json.Unmarshal(data, &manifest); err != nil {
				t.Fatal(err)
			}
			parts := manifest["parts"].([]any)
			part := parts[0].(map[string]any)
			path := filepath.Join(dir, part["name"].(string))
			rows, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch damage {
			case "active":
				part["closed"] = false
			case "retention":
				manifest["retention_truncated"] = true
			case "partial":
				rows = append(rows, []byte(`{"partial":`)...)
			case "corrupt":
				rows = append([]byte("broken JSON\n"), rows...)
			case "failed":
				rows = []byte(strings.Replace(string(rows), `"reason":"closed"`, `"reason":"failed","error":"synthetic failure"`, 1))
			case "lost_start":
				lines := strings.Split(string(rows), "\n")
				rows = []byte(strings.Join(append(lines[:1], lines[2:]...), "\n"))
			case "short_context":
				rows = []byte(strings.ReplaceAll(string(rows), `"post_until":"2026-11-07T12:00:00Z"`, `"post_until":"2026-11-07T12:00:30Z"`))
			}
			data, _ = json.Marshal(manifest)
			if err := os.WriteFile(manifestPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, rows, 0600); err != nil {
				t.Fatal(err)
			}
			s := milHTTPServer(t, dir)
			for _, endpoint := range []string{"report", "incident"} {
				w := milGET(s, "/api/mil/history/"+endpoint+"?session="+session+"&incident="+syntheticIncident)
				if damage == "lost_start" && endpoint == "report" {
					if w.Code != 409 {
						t.Fatal(w.Code, w.Body.String())
					}
					continue
				}
				var response struct {
					Incomplete bool                 `json:"incomplete"`
					Issues     []milstore.ReadIssue `json:"issues"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatal(err, w.Body.String())
				}
				if w.Code != 200 || !response.Incomplete || len(response.Issues) == 0 && !(damage == "short_context" && endpoint == "report") {
					t.Fatalf("lost surviving evidence/incompleteness: %d %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestMILExportRejectsOversizeBeforeWritingJSON(t *testing.T) {
	t.Parallel()
	dir, session := milHTTPArchive(t)
	parts, err := filepath.Glob(filepath.Join(dir, "mil-part-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var frame map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &frame); err != nil {
		t.Fatal(err)
	}
	frame["data"].(map[string]any)["fuel_context"] = map[string]any{"synthetic": strings.Repeat("x", 512<<10)}
	var rows []string
	rows = append(rows, lines[:2]...)
	for i := 0; i < 17; i++ {
		frame["sequence"] = 3 + i
		b, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, string(b))
	}
	for i, line := range lines[3:] {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		row["sequence"] = 20 + i
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, string(b))
	}
	if err := os.WriteFile(parts[0], []byte(strings.Join(rows, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	w := milGET(milHTTPServer(t, dir), "/api/mil/history/incident?session="+session+"&incident="+syntheticIncident)
	if w.Code != 413 || strings.HasPrefix(w.Body.String(), "{") {
		t.Fatalf("oversize export: %d %s", w.Code, w.Body.String())
	}
}
