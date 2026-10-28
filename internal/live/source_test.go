package live

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func readState(t *testing.T, s *Source, want telemetry.Health) telemetry.Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	for {
		got, err := s.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if got.Health == want {
			return got
		}
	}
}

func TestSourceLifecycleAndTerminalFailureKeepsReaderAlive(t *testing.T) {
	t.Parallel()
	cause := errors.New("adapter missing")
	s, err := New(probe.Config{Transport: "ble"}, func(context.Context) (elm327.Transport, error) { return nil, cause })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(t.Context()); !errors.Is(err, ErrNotStarted) {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); !errors.Is(err, ErrStarted) {
		t.Fatal(err)
	}
	select {
	case <-s.workerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not exhaust its retry budget")
	}
	got := readState(t, s, telemetry.Disconnected)
	if got.Source != telemetry.Live || got.RPM != nil || got.Transport != "ble_gatt" {
		t.Fatalf("fallback or stale data: %+v", got)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.Read(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("terminal stream ended HTTP source: %v", err)
	}
	if err := s.Close(); !errors.Is(err, cause) {
		t.Fatalf("lost terminal error: %v", err)
	}
	if _, err := s.Read(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
}

func TestSourceLatestOnlyAndImmutableSamples(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	s := newSource(func(ctx context.Context, sink func(record.Event) error) (probe.RecordingSummary, error) {
		now := time.Now().UTC()
		if err := sink(record.Event{Type: "capabilities", Data: probe.RecordingCapabilities{Report: probe.Report{Source: "elm327_ble"}}}); err != nil {
			return probe.RecordingSummary{}, err
		}
		for i := 0; i < 10000; i++ {
			log := probe.CommandLog{Command: "010C", Status: elm327.OK, CompletedAt: now}
			data := probe.RecordingCommand{CommandLog: log, Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(float64(i)), Unit: "rpm"}}}
			if err := sink(record.Event{Type: "command", Data: data}); err != nil {
				return probe.RecordingSummary{}, err
			}
		}
		close(ready)
		<-ctx.Done()
		return probe.RecordingSummary{Reason: "canceled"}, nil
	}, "ble")
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("slow reader blocked polling")
	}
	got := readState(t, s, telemetry.Connected)
	if got.RPM == nil || *got.RPM != 9999 {
		t.Fatalf("not latest: %+v", got)
	}
	*got.RPM = 1
	if s.latest.RPM == nil || *s.latest.RPM != 9999 {
		t.Fatal("reader mutated source cache")
	}
}

func TestEventsDoNotRefreshOtherSignalsAndGapClearsState(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	s.started = true
	consume := func(kind string, data any) {
		t.Helper()
		if err := s.consume(record.Event{Type: kind, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	consume("capabilities", probe.RecordingCapabilities{Report: probe.Report{Source: "elm327_ble"}})
	command := func(code string, value float64, status string) {
		t.Helper()
		consume("command", probe.RecordingCommand{CommandLog: probe.CommandLog{Command: code, CompletedAt: now, Status: elm327.OK}, Result: &probe.RecordingReading{Status: status, Measurement: elm327.Measurement{Value: new(value)}}})
	}
	command("0105", 80, "available")
	old := s.latest.Clone()
	now = now.Add(36 * time.Second)
	command("010C", 900, "available")
	got := s.latest.At(now, 3*time.Second)
	if got.RPM == nil || got.CoolantC != nil || !s.latest.Signals["coolant_c"].CapturedAt.Equal(old.Signals["coolant_c"].CapturedAt) {
		t.Fatalf("timestamp refresh: %+v", got)
	}
	command("010C", 999, "temporarily_unavailable")
	if s.latest.RPM != nil {
		t.Fatal("NO DATA preserved value")
	}
	consume("gap", map[string]any{"error": "timeout"})
	if s.latest.Health != telemetry.Disconnected || s.latest.RPM != nil || s.latest.CoolantC != nil {
		t.Fatal("gap retained data")
	}
	consume("segment_start", nil)
	if s.latest.Health != telemetry.Connecting || s.latest.Signals["rpm"].Support != "not_yet_tested" {
		t.Fatal("old capabilities survived reconnect")
	}
}

func TestCloseAllowsFinalLifecycleEvents(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	s := newSource(func(ctx context.Context, sink func(record.Event) error) (probe.RecordingSummary, error) {
		close(ready)
		<-ctx.Done()
		if err := sink(record.Event{Type: "segment_end"}); err != nil {
			return probe.RecordingSummary{}, err
		}
		if err := sink(record.Event{Type: "recording_end"}); err != nil {
			return probe.RecordingSummary{}, err
		}
		return probe.RecordingSummary{Reason: "canceled"}, nil
	}, "ble")
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-ready
	if err := s.Close(); err != nil {
		t.Fatalf("intentional Close became a sink failure: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLiveDiagnosticCountersSurviveSegmentReset(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	for _, event := range []record.Event{
		{Type: "command", Data: probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "010C", Status: elm327.NoData}, Result: &probe.RecordingReading{Status: "temporarily_unavailable"}}},
		{Type: "gap"}, {Type: "recovery"}, {Type: "segment_start"},
	} {
		if err := s.consume(event); err != nil {
			t.Fatal(err)
		}
	}
	got := s.latest.Diagnostics
	if got.Commands != 1 || got.Errors != 1 || got.Gaps != 1 || got.Recoveries != 1 {
		t.Fatalf("lost bounded diagnostic counters: %+v", got)
	}
}
