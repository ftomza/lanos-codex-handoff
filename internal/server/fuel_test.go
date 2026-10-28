package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestServerHasOnePersistentFuelComputerAndExpiresWithoutSourceUpdates(t *testing.T) {
	t.Parallel()
	cfg := fuel.DefaultConfig()
	cfg.StaleAfter = 100 * time.Millisecond
	c, err := fuel.Open(cfg, telemetry.Simulator, filepath.Join(t.TempDir(), "fuel.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{SourceName: telemetry.Simulator, StaleAfter: cfg.StaleAfter, Fuel: c})
	if err != nil {
		t.Fatal(err)
	}
	source := &oneFuelSample{sample: sim.Sample(time.Now(), time.Now())}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx, source) }()
	deadline := time.Now().Add(time.Second)
	for s.snapshot().Fuel == nil || s.snapshot().Fuel.FlowLPH == nil {
		if time.Now().After(deadline) {
			t.Fatal("no fuel view")
		}
		time.Sleep(time.Millisecond)
	}
	first := s.snapshot().Fuel
	for range 10 {
		if s.snapshot().Fuel.Liters != first.Liters {
			t.Fatal("HTTP read integrated fuel")
		}
	}
	time.Sleep(120 * time.Millisecond)
	if v := s.snapshot().Fuel; v.FlowLPH != nil || v.Status == "available" {
		t.Fatalf("fuel did not expire: %+v", v)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("fuel shutdown blocked")
	}
}

type oneFuelSample struct {
	sample telemetry.Snapshot
	sent   bool
}

func (s *oneFuelSample) Start(context.Context) error { return nil }
func (s *oneFuelSample) Read(ctx context.Context) (telemetry.Snapshot, error) {
	if !s.sent {
		s.sent = true
		return s.sample, nil
	}
	<-ctx.Done()
	return telemetry.Snapshot{}, ctx.Err()
}
func (s *oneFuelSample) Close() error { return nil }

func TestEmbeddedFuelModulesAreServed(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	for _, path := range []string{"/fuel.mjs", "/fuel-controls.mjs"} {
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, httptest.NewRequest(http.MethodGet, path, nil))
		if out.Code != http.StatusOK {
			t.Fatalf("missing embedded module %s: %d", path, out.Code)
		}
	}
}
