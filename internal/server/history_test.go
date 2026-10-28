package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuelhistory"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestHistoryAPIUsesOnlyConfiguredSessions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := fuelhistory.Open(fuelhistory.DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{HistoryDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/fuel/history/sessions", "/api/fuel/history/report"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 200 || !json.Valid(w.Body.Bytes()) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, query := range []string{"session=../escape", "points=100000", "from=bad", "path=/etc/passwd"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/fuel/history/report?"+query, nil))
		if w.Code != 400 {
			t.Fatalf("unsafe query accepted %s: %d", query, w.Code)
		}
	}
	w := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/fuel/history/sessions", nil))
	if w.Code != 503 {
		t.Fatal("history implicitly enabled")
	}
}

func TestRecordingFailureDoesNotStopSourceOrHTTP(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := fuelhistory.Open(fuelhistory.DefaultConfig(filepath.Join(dir, "history"), telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	c, err := fuel.Open(fuel.DefaultConfig(), telemetry.Simulator, filepath.Join(dir, "fuel.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if err := c.SetDiagnosticObserver(time.Second, false, r.Submit); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Fuel: c, Recorder: r, HistoryDir: filepath.Join(dir, "history")})
	if err != nil {
		t.Fatal(err)
	}
	source, err := sim.New(10 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, source) }()
	// An unencodable row causes a real asynchronous writer failure on disk output.
	d := fuel.Diagnostic{Kind: "sample", At: time.Now(), Snapshot: telemetry.Snapshot{Source: telemetry.Simulator, Message: strings.Repeat("x", fuelhistory.MaxLine)}}
	if err := r.Submit(d); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for r.Status().State != "failed" || s.snapshot().Fuel.DiagnosticsError == "" {
		if time.Now().After(deadline) {
			t.Fatal("runtime recording failure not exposed")
		}
		time.Sleep(time.Millisecond)
	}
	previous := s.snapshot().CapturedAt
	for !s.snapshot().CapturedAt.After(previous) {
		if time.Now().After(deadline) {
			t.Fatal("source stopped after recorder failure")
		}
		time.Sleep(time.Millisecond)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
	if w.Code != http.StatusOK {
		t.Fatal("HTTP stopped after recorder failure", w.Code)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("shutdown lost recorder error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown blocked")
	}
}
