package fuelhistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func diagnostic(at time.Time) fuel.Diagnostic {
	return fuel.Diagnostic{Kind: "sample", At: at, Snapshot: telemetry.Snapshot{Source: telemetry.Simulator, Health: telemetry.Connected}, Fuel: telemetry.Fuel{Factor: 1, Revision: 1, ResetAt: at, Settings: telemetry.FuelSettings{VE: .75}}}
}
func TestWriterPersistsPrivateHistoryAndRestarts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(DefaultConfig(dir, telemetry.Simulator)); err == nil {
		t.Fatal("two writers")
	}
	d := diagnostic(time.Now())
	if err := r.Submit(d); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	rows := 0
	if _, err := Read(t.Context(), dir, Options{}, func(e Entry) error { rows++; return nil }); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("rows=%d", rows)
	}
	sessions, err := List(dir)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list: %v %+v", err, sessions)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		info, _ := e.Info()
		if info.Mode().Perm() != 0600 {
			t.Fatalf("public file %s", e.Name())
		}
	}
	r, err = Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	sessions, err = List(dir)
	if err != nil || len(sessions) != 2 {
		t.Fatal(err, sessions)
	}
	if _, err := Open(DefaultConfig(dir, telemetry.Live)); err == nil {
		t.Fatal("mixed sources")
	}
}
func TestRotationDeletesOnlyOwnedParts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := DefaultConfig(dir, telemetry.Simulator)
	cfg.MaxFiles = 2
	cfg.MaxBytes = 1 << 20
	foreign := filepath.Join(dir, "owner-notes.txt")
	if err := os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	d := diagnostic(time.Now())
	d.Snapshot.Message = strings.Repeat("x", 30000)
	for range 120 {
		if err := r.Submit(d); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := loadManifest(dir)
	if err != nil || len(m.Parts) != 2 || !m.Truncated {
		t.Fatal(err, m)
	}
	for _, p := range m.Parts {
		st, err := os.Stat(filepath.Join(dir, p.Name))
		if err != nil || st.Size() > cfg.MaxBytes {
			t.Fatal(err, st)
		}
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatal("deleted foreign file")
	}
}
func TestReaderReportsPartialTailAndRejectsManifestTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(dir, m.Parts[0].Name), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(`{"version":1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	issues, err := Read(t.Context(), dir, Options{}, func(Entry) error { return nil })
	if err != nil || len(issues) == 0 {
		t.Fatal("tail hidden", err, issues)
	}
	m.Parts[0].Name = "../outside.jsonl"
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := List(dir); err == nil {
		t.Fatal("unsafe manifest accepted")
	}
}
func TestReaderCancellationAndSinkFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Read(ctx, dir, Options{}, func(Entry) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	cause := errors.New("sink failed")
	if _, err := Read(t.Context(), dir, Options{}, func(Entry) error { return cause }); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}

func TestMissingOwnedPartDoesNotResetManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, m.Parts[0].Name)); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(DefaultConfig(dir, telemetry.Simulator)); err == nil {
		t.Fatal("silently reset missing part")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if string(after) != string(before) {
		t.Fatal("modified corrupt catalog")
	}
}
func TestRuntimeWriteFailureIsVisibleAndCloseReturnsIt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.disk.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(diagnostic(time.Now())); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for r.Status().State != "failed" {
		if time.Now().After(deadline) {
			t.Fatal("hidden write failure")
		}
		time.Sleep(time.Millisecond)
	}
	if err := r.Close(); err == nil {
		t.Fatal("clean close after failed write")
	}
	if _, err := os.Stat(filepath.Join(dir, ".writer.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("lock leaked", err)
	}
}
func TestQueueOverflowStopsRecordingExplicitly(t *testing.T) {
	t.Parallel()
	r := &Recorder{disk: &diskWriter{cfg: DefaultConfig(t.TempDir(), telemetry.Simulator), session: strings.Repeat("a", 32)}, queue: make(chan Entry, 1), status: Status{State: "recording"}}
	if err := r.Submit(diagnostic(time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(diagnostic(time.Now())); err == nil || r.Status().State != "failed" {
		t.Fatal("silently dropped row")
	}
}

func TestManifestRejectsReusedOrExhaustedPartNumbers(t *testing.T) {
	t.Parallel()
	for _, next := range []uint64{0, ^uint64(0)} {
		t.Run(fmt.Sprint(next), func(t *testing.T) {
			t.Parallel()
			dir := syntheticHistory(t, []Entry{{Kind: "session_start", At: time.Now()}})
			m, err := loadManifest(dir)
			if err != nil {
				t.Fatal(err)
			}
			m.Next = next
			if err := saveManifest(dir, m); err != nil {
				t.Fatal(err)
			}
			if _, err := loadManifest(dir); err == nil {
				t.Fatal("accepted invalid part sequence")
			}
		})
	}
}

func TestRecorderPersistsFailureCauseWhenStorageStillWorks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := Open(DefaultConfig(dir, telemetry.Simulator))
	if err != nil {
		t.Fatal(err)
	}
	d := diagnostic(time.Now())
	d.Snapshot.Source = telemetry.Live
	if err := r.Submit(d); err == nil {
		t.Fatal("accepted mixed source")
	}
	if err := r.Close(); err == nil {
		t.Fatal("failure hidden")
	}
	report, err := Analyze(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, issue := range report.Issues {
		found = found || issue.Code == "recording_failed" && strings.Contains(issue.Detail, "source mismatch")
	}
	if !found || !report.Incomplete {
		t.Fatal("recording failure cause lost", report.Issues)
	}
}
