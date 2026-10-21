package sim

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestDrivingCycle(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		elapsed    time.Duration
		wantSpeed  float64
		wantHealth telemetry.Health
	}{
		{"idle", 0, 0, telemetry.Connected},
		{"accelerating", 15 * time.Second, 40, telemetry.Connected},
		{"cruising", 30 * time.Second, 80, telemetry.Connected},
		{"decelerating", 40 * time.Second, 40, telemetry.Connected},
		{"disconnect", 50 * time.Second, 0, telemetry.Disconnected},
		{"reconnected", 56 * time.Second, 0, telemetry.Connected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := start.Add(tc.elapsed)
			got := Sample(start, now)
			if !reflect.DeepEqual(got, Sample(start, now)) {
				t.Fatal("sample must be deterministic")
			}
			if got.Source != telemetry.Simulator || got.Health != tc.wantHealth || !got.ReceivedAt.Equal(now) {
				t.Fatalf("unexpected metadata: %+v", got)
			}
			if tc.wantHealth == telemetry.Disconnected {
				if got.SpeedKMH != nil || got.RPM != nil || got.CoolantC != nil || got.VoltageV != nil {
					t.Fatal("disconnected sample must not fabricate measurements")
				}
				return
			}
			if got.SpeedKMH == nil || *got.SpeedKMH != tc.wantSpeed || got.RPM == nil || *got.RPM < 750 || *got.RPM > 4500 {
				t.Fatalf("unrealistic sample: %+v", got)
			}
		})
	}
}

func TestUnavailableVoltage(t *testing.T) {
	t.Parallel()
	start := time.Now().UTC()
	s := Sample(start, start.Add(25*time.Second))
	if s.VoltageV != nil || s.SpeedKMH == nil || s.Health != telemetry.Connected {
		t.Fatalf("voltage availability must be independent: %+v", s)
	}
}

func TestLifecycle(t *testing.T) {
	t.Parallel()
	s, err := New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var source telemetry.Source = s
	if _, err := source.Read(t.Context()); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("read before start: %v", err)
	}
	lifecycle, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := source.Start(lifecycle); err != nil {
		t.Fatal(err)
	}
	if err := source.Start(lifecycle); !errors.Is(err, ErrStarted) {
		t.Fatalf("duplicate start: %v", err)
	}
	if _, err := source.Read(lifecycle); err != nil {
		t.Fatal(err)
	}
	readCtx, cancelRead := context.WithCancel(t.Context())
	cancelRead()
	if _, err := source.Read(readCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read: %v", err)
	}
	cancel()
	if _, err := source.Read(t.Context()); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lifecycle: %v", err)
	}
}

func TestCloseUnblocksRead(t *testing.T) {
	t.Parallel()
	s, err := New(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(t.Context()); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := s.Read(t.Context()); result <- err }()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("close read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not unblock read")
	}
	if err := s.Start(t.Context()); !errors.Is(err, ErrClosed) {
		t.Fatalf("restart closed source: %v", err)
	}
}

func TestInvalidInterval(t *testing.T) {
	t.Parallel()
	for _, interval := range []time.Duration{0, -time.Second} {
		if _, err := New(interval); err == nil {
			t.Fatalf("accepted interval %s", interval)
		}
	}
}
