package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestHistoryFlagsAreOptInAndBounded(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--fuel-record-interval=10s"}, {"--fuel-record-dir=x", "--fuel=off"}, {"--fuel-record-dir=x", "--fuel-history-dir=y"}, {"--fuel-record-dir=x", "--fuel-record-max-bytes=1"}, {"--fuel-record-dir=x", "--fuel-record-max-files=33"}, {"--fuel-record-cold-start"}} {
		if _, err := parseDashboardFlags(args, &bytes.Buffer{}); err == nil {
			t.Fatal("accepted", args)
		}
	}
	dir := filepath.Join(t.TempDir(), "history")
	o, err := parseDashboardFlags([]string{"--fuel-record-dir=" + dir, "--fuel-state=" + filepath.Join(t.TempDir(), "sim.json")}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := openFuel(o, telemetry.Simulator)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	r, path, err := openHistory(o, telemetry.Simulator, c)
	if err != nil {
		t.Fatal(err)
	}
	if path != dir || r == nil || o.recordInterval != time.Second {
		t.Fatal("no opt-in recorder")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryOpenFailurePrecedesSourceStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "blocked")
	if err := os.WriteFile(path, []byte("owner file"), 0600); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	deps := dashboardDependencies{live: func(probe.Config) (telemetry.Source, error) {
		return &terminalSource{started: started}, nil
	}}
	err := runWith(t.Context(), []string{"--source=elm327", "--listen=127.0.0.1:0", "--fuel-state=" + filepath.Join(dir, "live.json"), "--fuel-record-dir=" + path}, &bytes.Buffer{}, deps)
	if err == nil {
		t.Fatal("history startup failure hidden")
	}
	select {
	case <-started:
		t.Fatal("opened hardware before history")
	default:
	}
}
