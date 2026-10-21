package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{SourceName: telemetry.Simulator, StaleAfter: 50 * time.Millisecond, StreamInterval: 5 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotFreshnessAndAssets(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	now := time.Now().UTC()
	s.publish(sim.Sample(now, now.Add(-time.Second)))
	for _, tc := range []struct {
		path, content string
		status        int
	}{
		{"/", "SIMULATOR", http.StatusOK},
		{"/app.mjs", "EventSource", http.StatusOK},
		{"/style.css", "@media", http.StatusOK},
		{"/missing", "404", http.StatusNotFound},
		{"/api/snapshot", `"health":"stale"`, http.StatusOK},
	} {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if r.Code != tc.status || !strings.Contains(r.Body.String(), tc.content) {
			t.Errorf("GET %s: status %d, body %s", tc.path, r.Code, r.Body.String())
		}
	}
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodPost, "/events", nil))
	if r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST events = %d", r.Code)
	}
}

func readEvent(t *testing.T, scanner *bufio.Scanner) Event {
	t.Helper()
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var event Event
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				t.Fatal(err)
			}
			return event
		}
	}
	t.Fatalf("stream ended without event: %v", scanner.Err())
	return Event{}
}

func TestSSEStaleAndReconnect(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	now := time.Now().UTC()
	s.publish(sim.Sample(now, now))
	httpServer := httptest.NewServer(s.Handler())
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	open := func() *http.Response {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			t.Fatal("missing SSE content type")
		}
		return resp
	}
	resp := open()
	scanner := bufio.NewScanner(resp.Body)
	event := readEvent(t, scanner)
	if event.Source != telemetry.Simulator || event.StaleAfterMS != 50 {
		t.Fatalf("bad initial event: %+v", event)
	}
	for event.Health != telemetry.Stale {
		event = readEvent(t, scanner)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	now = time.Now().UTC()
	s.publish(sim.Sample(now, now.Add(50*time.Second)))
	reconnected := open()
	defer reconnected.Body.Close()
	event = readEvent(t, bufio.NewScanner(reconnected.Body))
	if event.Health != telemetry.Disconnected || event.SpeedKMH != nil {
		t.Fatalf("reconnect must get latest: %+v", event)
	}
}

func TestRunSourceCancellation(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	source, err := sim.New(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- s.Run(ctx, source) }()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for s.snapshot().Health != telemetry.Connected {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("source never published")
		}
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not stop")
	}
	if _, err := source.Read(t.Context()); err == nil {
		t.Fatal("Run must close source")
	}
}

func TestInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{{StaleAfter: -time.Second}, {StreamInterval: -time.Second}} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("accepted invalid config: %+v", cfg)
		}
	}
}
