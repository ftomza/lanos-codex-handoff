package milstore

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRotationRetentionBounds(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	foreign := filepath.Join(dir, "unrelated.jsonl")
	if err := os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(dir, "simulator")
	cfg.MaxBytes = 2 << 20
	cfg.MaxFiles = 2
	r, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{"padding":"` + strings.Repeat("x", 700000) + `"}`)
	for i := range 10 {
		if err := r.Submit(Entry{Kind: "sample", Data: data}); err != nil {
			t.Fatal(err)
		}
		want := uint64(i + 2)
		waitFor(t, func() bool { return r.Status().Rows >= want })
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !r.Status().RetentionTruncated {
		t.Fatal("retired context not reported")
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 2 || !m.Truncated {
		t.Fatal(m)
	}
	for _, p := range m.Parts {
		info, err := os.Stat(filepath.Join(dir, p.Name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > cfg.MaxBytes || !p.Closed {
			t.Fatal(info, p)
		}
	}
	rows := readRows(t, dir)
	if rows[0].Sequence <= 1 || rows[len(rows)-1].Kind != "session_end" {
		t.Fatal("retained sequence/footer incorrect")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Sequence != rows[i-1].Sequence+1 {
			t.Fatal("noncontiguous retained sequence")
		}
	}
	data, err = os.ReadFile(foreign)
	if err != nil || string(data) != "keep" {
		t.Fatalf("foreign file modified: %v", err)
	}
	r, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Status().RetentionTruncated {
		t.Fatal("retention flag lost on restart")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRowLimitsAndFailureFooter(t *testing.T) {
	t.Parallel()
	for _, size := range []int{MaxLine - 100, MaxLine + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			t.Parallel()
			dir := realTemp(t)
			r, err := Open(DefaultConfig(dir, "live"))
			if err != nil {
				t.Fatal(err)
			}
			err = r.Submit(Entry{Kind: "sample", Data: json.RawMessage(`"` + strings.Repeat("x", size) + `"`)})
			if err == nil {
				waitFor(t, func() bool { return r.Status().State == "failed" })
			}
			if err := r.Close(); err == nil {
				t.Fatal("oversize did not fail")
			}
			rows := readRows(t, dir)
			if err != nil && (len(rows) != 2 || rows[1].Kind != "session_end") {
				t.Fatal("still-writable failure footer missing")
			}
			for _, row := range rows {
				if row.Kind == "session_end" && !strings.Contains(string(row.Data), "recording_failed") {
					t.Fatal("failure footer claims clean close")
				}
			}
		})
	}
}

func TestRotationCollisionPreservesForeignFileAndReportsRetirement(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	cfg := DefaultConfig(dir, "live")
	cfg.MaxBytes = 2 << 20
	cfg.MaxFiles = 1
	r, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(dir, "mil-part-0000000000000002.jsonl")
	if err := os.WriteFile(foreign, []byte("foreign collision"), 0600); err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`"` + strings.Repeat("x", 700000) + `"`)
	for i := range 2 {
		if err := r.Submit(Entry{Kind: "sample", Data: data}); err != nil {
			t.Fatal(err)
		}
		want := uint64(i + 2)
		waitFor(t, func() bool { return r.Status().Rows == want })
	}
	if err := r.Submit(Entry{Kind: "sample", Data: data}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Status().State == "failed" })
	if err := r.Close(); err == nil {
		t.Fatal("collision was not reported")
	}
	if !r.Status().RetentionTruncated {
		t.Fatal("retired context not reported after collision")
	}
	got, err := os.ReadFile(foreign)
	if err != nil || string(got) != "foreign collision" {
		t.Fatalf("collision was modified: %q %v", got, err)
	}
}

func TestCrashPreservesPartialSession(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashWorker$")
	cmd.Env = append(os.Environ(), "MILSTORE_CRASH_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("crash helper: %v %s", err, out)
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Parts) != 1 || m.Parts[0].Closed {
		t.Fatal("crash marked clean", m)
	}
	rows := readRows(t, dir)
	if len(rows) != 2 || rows[1].Kind != "incident" {
		t.Fatal(rows)
	}
	if r, err := Open(DefaultConfig(dir, "live")); err == nil {
		r.Close()
		t.Fatal("stale crash lock ignored")
	}
	// This is an isolated synthetic process; simulate explicit operator lock recovery.
	if err := os.Remove(filepath.Join(dir, lockName)); err != nil {
		t.Fatal(err)
	}
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Parts[0].Closed || !m.Parts[0].Aborted {
		t.Fatal("restart falsely closed crashed session", m)
	}
}

func TestCrashWorker(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("MILSTORE_CRASH_DIR")
	if dir == "" {
		t.Skip("subprocess only")
	}
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(Entry{Kind: "incident", At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Status().Rows == 2 })
	os.Exit(0)
}
