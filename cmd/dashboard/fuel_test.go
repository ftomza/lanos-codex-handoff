package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestFuelConfiguration(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--fuel=wat"}, {"--fuel-ve=NaN"}, {"--fuel-density=0"}, {"--fuel-model=injector"}, {"--fuel-low-speed=0"}, {"--fuel=off", "--fuel-reset-state"}} {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()
			if _, err := parseDashboardFlags(args, &bytes.Buffer{}); err == nil {
				t.Fatalf("bad fuel config accepted: %v", args)
			}
		})
	}
	got, err := parseDashboardFlags([]string{"--source=elm327", "--fuel-allow-unverified-iat"}, &bytes.Buffer{})
	if err != nil || got.fuelMode != "auto" || !got.fuelConfig.AllowUnverifiedIAT {
		t.Fatal("fuel defaults/opt-in", err)
	}
}
func TestFuelStateFailurePrecedesHardwareStart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	started := false
	deps := dashboardDependencies{listen: func(ctx context.Context, _, _ string) (net.Listener, error) {
		return (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	}, live: func(probe.Config) (telemetry.Source, error) { return &startTracker{started: &started}, nil }}
	err := runWith(t.Context(), []string{"--source=elm327", "--fuel-state=" + path}, &bytes.Buffer{}, deps)
	if err == nil || started {
		t.Fatalf("corrupt state started hardware: %v %t", err, started)
	}
}

type startTracker struct{ started *bool }

func (s *startTracker) Start(context.Context) error {
	*s.started = true
	return errors.New("unexpected start")
}
func (s *startTracker) Read(context.Context) (telemetry.Snapshot, error) {
	return telemetry.Snapshot{}, errors.New("unexpected read")
}
func (s *startTracker) Close() error { return nil }
