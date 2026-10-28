package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/live"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

// Saved physical response bytes plus explicitly synthetic ATRV/continuation.
// Access to pending is serialized by the real ELM Session, as on the adapter.
type liveFixture struct {
	responses map[string]string
	pending   []byte
	mu        sync.Mutex
	commands  []string
	fail      atomic.Bool
	closed    atomic.Int32
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()
	data, err := os.ReadFile("../elm327/testdata/vgate-2026-10-24.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Exchanges []struct{ Command, Raw string }
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	a := &liveFixture{responses: make(map[string]string, len(f.Exchanges)+2)}
	for _, x := range f.Exchanges {
		a.responses[x.Command] = x.Raw
	}
	a.responses["ATRV"] = "14.2V\r>"
	a.responses["0120"] = "4120C0000000\r>"
	return a
}

func (a *liveFixture) Write(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if a.fail.Load() {
		return 0, io.EOF
	}
	command := strings.TrimSuffix(string(p), "\r")
	a.mu.Lock()
	a.commands = append(a.commands, command)
	a.mu.Unlock()
	response, ok := a.responses[command]
	if !ok {
		response = "NO DATA\r>"
	}
	a.pending = append(a.pending, response...)
	return len(p), nil
}

func (a *liveFixture) Read(ctx context.Context, p []byte) (int, error) {
	if len(a.pending) == 0 {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	// Deliberately fragment notifications independently of response boundaries.
	n := copy(p, a.pending[:min(3, len(a.pending))])
	a.pending = a.pending[n:]
	return n, nil
}

func (a *liveFixture) Close() error { a.closed.Add(1); return nil }

func openLiveEvents(t *testing.T, ctx context.Context, client *http.Client, url string) *bufio.Scanner {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return bufio.NewScanner(resp.Body)
}

func TestLiveELMStreamSharedByBrowsersAndSurvivesTerminalFailure(t *testing.T) {
	t.Parallel()
	a := newLiveFixture(t)
	var opens atomic.Int32
	source, err := live.New(probe.Config{Transport: "ble"}, func(context.Context) (elm327.Transport, error) {
		opens.Add(1)
		return a, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{SourceName: telemetry.Live, StreamInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, source) }()
	host := httptest.NewServer(s.Handler())
	t.Cleanup(host.Close)
	clients := []*bufio.Scanner{
		openLiveEvents(t, ctx, host.Client(), host.URL),
		openLiveEvents(t, ctx, host.Client(), host.URL),
	}
	for _, client := range clients {
		event := readEvent(t, client)
		for event.Health != telemetry.Connected {
			event = readEvent(t, client)
		}
		if event.Source != telemetry.Live || event.RPM == nil || *event.RPM != 0 || event.CoolantC == nil || *event.CoolantC != 78 {
			t.Fatalf("bad physical-fixture snapshot: %+v", event)
		}
		if event.Signals["voltage_v"].Source != "adapter_atrv" || event.Protocol != "A5" {
			t.Fatal("lost source provenance")
		}
	}
	a.fail.Store(true)
	for _, client := range clients {
		event := readEvent(t, client)
		for event.Health != telemetry.Disconnected || !strings.Contains(event.Message, "Перезапустите") {
			event = readEvent(t, client)
		}
		if event.RPM != nil || event.VoltageV != nil {
			t.Fatal("terminal failure retained values")
		}
	}
	select {
	case err := <-done:
		t.Fatalf("terminal failure stopped server source: %v", err)
	default:
	}
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
	if r.Code != http.StatusOK || opens.Load() != 1 {
		t.Fatal("HTTP unavailable or browser started another adapter")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("lost cleanup/terminal evidence: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("live source did not join worker")
	}
	if a.closed.Load() != 1 {
		t.Fatal("transport not closed exactly once")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, blocked := range []string{"ATZ", "0110", "012F", "0140", "0142", "015E", "04", "0902"} {
		if slices.Contains(a.commands, blocked) {
			t.Fatalf("unsupported or forbidden command: %s", blocked)
		}
	}
}

func TestLiveSnapshotIndependentExpiry(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	now := time.Now().UTC()
	old := now.Add(-36 * time.Second)
	s.publish(telemetry.Snapshot{Source: telemetry.Live, Health: telemetry.Connected, CapturedAt: now, ReceivedAt: now,
		Signals: map[string]telemetry.Signal{
			"rpm":       {Value: new(900.0), Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
			"coolant_c": {Value: new(90.0), Status: "available", CapturedAt: old, ReceivedAt: old, StaleAfterMS: 35000},
		}})
	r := httptest.NewRecorder()
	s.Handler().ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
	var event Event
	if err := json.Unmarshal(r.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	if event.Health != telemetry.Connected || event.RPM == nil || event.CoolantC != nil || event.Signals["coolant_c"].Status != "stale" {
		t.Fatalf("one signal refreshed another: %+v", event)
	}
}
