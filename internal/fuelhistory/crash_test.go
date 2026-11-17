package fuelhistory

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestAbruptProcessExitKeepsCompletePrefixAndExclusiveLock(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestHistoryAbruptExitHelper$")
	cmd.Env = append(os.Environ(), "LANOS_HISTORY_CRASH_TEST="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, output)
	}
	r, err := Analyze(t.Context(), dir, Options{})
	if err != nil || r.Samples != 1 || !r.Incomplete {
		t.Fatal("lost complete prefix after abrupt exit", err, r)
	}
	if _, err := Open(DefaultConfig(dir, telemetry.Simulator)); err == nil {
		t.Fatal("stale lock silently removed")
	}
	// The test owns this catalog and has waited for its only writer to exit.
	if err := os.Remove(filepath.Join(dir, ".writer.lock")); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	sessions, err := List(dir)
	if err != nil || len(sessions) != 2 {
		t.Fatal("restart overwrote previous session", err, sessions)
	}
}

func TestHistoryAbruptExitHelper(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("LANOS_HISTORY_CRASH_TEST")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(diagnostic(time.Now())); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for r.Status().Rows < 1 {
		if time.Now().After(deadline) {
			t.Fatal("child did not save sample")
		}
		time.Sleep(time.Millisecond)
	}
	if err := r.disk.file.Sync(); err != nil {
		t.Fatal(err)
	}
	os.Exit(0) // Deliberately bypass recorder.Close: no session_end or lock cleanup.
}
