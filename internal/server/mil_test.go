package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type syntheticMILSource struct {
	withFuel       bool
	observer       func(telemetry.Snapshot)
	latest         telemetry.Snapshot
	delivered      bool
	starts, closes atomic.Int32
	ready          chan struct{}
}

func (s *syntheticMILSource) SetObserver(fn func(telemetry.Snapshot)) error {
	if s.starts.Load() != 0 || s.observer != nil {
		return fmt.Errorf("observer attached too late")
	}
	s.observer = fn
	return nil
}
func (s *syntheticMILSource) Start(context.Context) error {
	s.starts.Add(1)
	for _, on := range []bool{false, true, false} {
		now := time.Now()
		s.latest = telemetry.Snapshot{Source: telemetry.Live, Generation: 1, Health: telemetry.Connected, CapturedAt: now, ReceivedAt: now,
			MIL: &telemetry.MILReading{On: new(on), DTCCount: new(0), Status: "available", RawResponse: "synthetic", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 15000}}
		if s.withFuel {
			s.latest.Signals = map[string]telemetry.Signal{
				"fuel_rate_lph": {Value: new(3.6), Unit: "L/h", Source: "ecu_mode01", Support: "supported", Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
				"speed_kmh":     {Value: new(36.0), Unit: "km/h", Source: "ecu_mode01", Support: "supported", Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
			}
		}
		if s.observer != nil {
			s.observer(s.latest.Clone())
		}
	}
	close(s.ready)
	return nil
}

type failedMILWriter struct{ err error }

func (w failedMILWriter) Submit(milstore.Entry) error { return w.err }
func (w failedMILWriter) Status() milstore.Status {
	return milstore.Status{State: "failed", Error: w.err.Error()}
}
func (w failedMILWriter) Close() error { return w.err }

func TestMILFailureIsVisibleWhileTelemetryAndFuelContinue(t *testing.T) {
	t.Parallel()
	cause := errors.New("synthetic mil disk failure")
	computer, err := fuel.Open(fuel.DefaultConfig(), telemetry.Live, filepath.Join(t.TempDir(), "fuel.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	capture := mil.NewCapture(failedMILWriter{cause}, func(now time.Time) (json.RawMessage, error) { return json.Marshal(computer.Diagnostic(now)) }, 3*time.Second)
	app, err := New(Config{SourceName: telemetry.Live, Fuel: computer, MIL: capture})
	if err != nil {
		t.Fatal(err)
	}
	source := &syntheticMILSource{ready: make(chan struct{}), withFuel: true}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, source) }()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("source stopped on recording failure: %v", err)
		case <-deadline.C:
			t.Fatal("fuel did not advance after mil recording failed")
		case <-ticker.C:
			e := app.snapshot()
			if e.Fuel == nil || e.Fuel.Liters <= 0 {
				continue
			}
			if e.Health != telemetry.Connected || e.MILRecording == nil || e.MILRecording.State != "failed" {
				t.Fatal(e)
			}
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/snapshot", nil))
			var api Event
			if err := json.Unmarshal(response.Body.Bytes(), &api); err != nil {
				t.Fatal(err)
			}
			if api.MILRecording == nil || api.MILRecording.Error == "" || api.Fuel.Liters <= 0 {
				t.Fatal(api)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, cause) {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("failed capture shutdown blocked")
			}
			return
		}
	}
}
func (s *syntheticMILSource) Read(ctx context.Context) (telemetry.Snapshot, error) {
	if !s.delivered {
		s.delivered = true
		return s.latest.Clone(), nil
	}
	<-ctx.Done()
	return telemetry.Snapshot{}, ctx.Err()
}
func (s *syntheticMILSource) Close() error { s.closes.Add(1); return nil }

func TestMILCaptureUsesOneObserverAcrossTwoBrowsers(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := milstore.Open(milstore.DefaultConfig(dir, telemetry.Live))
	if err != nil {
		t.Fatal(err)
	}
	capture := mil.NewCapture(w, nil, 3*time.Second)
	app, err := New(Config{SourceName: telemetry.Live, MIL: capture, StreamInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	source := &syntheticMILSource{ready: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx, source) }()
	select {
	case <-source.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("source did not start")
	}
	httpServer := httptest.NewServer(app.Handler())
	defer httpServer.Close()
	for range 2 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/events", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := httpServer.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		e := readEvent(t, bufio.NewScanner(resp.Body))
		if e.MILRecording == nil || e.MILRecording.LastEvent.Kind != "MIL_OFF" {
			t.Fatal(e)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("capture shutdown blocked")
	}
	if source.starts.Load() != 1 || source.closes.Load() != 1 {
		t.Fatal("multiple source lifecycles")
	}
	files, err := filepath.Glob(filepath.Join(dir, "mil-part-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	f, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	events := make([]string, 0, 3)
	for scanner.Scan() {
		var row struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row.Kind == "event" {
			var e mil.Event
			if err := json.Unmarshal(row.Data, &e); err != nil {
				t.Fatal(err)
			}
			events = append(events, e.Kind)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(events) != "[MIL_INITIAL_OFF MIL_ON MIL_OFF]" {
		t.Fatal("transition lost or duplicated by browser", events)
	}
}
