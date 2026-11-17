package milstore

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLockAndConcurrentClose(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig(realTemp(t), "simulator")
	r, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(cfg); err == nil {
		other.Close()
		t.Fatal("second writer acquired lock")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := r.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := r.Submit(Entry{Kind: "late"}); err == nil {
		t.Fatal("accepted after close")
	}
	r, err = Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsafeFiles(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"directory_symlink", "parent_symlink", "manifest_symlink", "part_symlink", "orphan", "bad_manifest", "source", "missing_part", "traversal"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			dir := realTemp(t)
			cfg := DefaultConfig(dir, "live")
			foreign := filepath.Join(dir, "foreign.txt")
			if err := os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "directory_symlink" || kind == "parent_symlink" {
				link := filepath.Join(realTemp(t), "link")
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				cfg.Dir = link
				if kind == "parent_symlink" {
					cfg.Dir = filepath.Join(link, "nested")
				}
			} else if kind == "manifest_symlink" {
				if err := os.Symlink(foreign, filepath.Join(dir, manifestName)); err != nil {
					t.Fatal(err)
				}
			} else if kind == "orphan" {
				if err := os.WriteFile(filepath.Join(dir, "mil-part-0000000000000001.jsonl"), []byte("orphan"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if kind == "bad_manifest" {
				if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(`{broken`), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				r, err := Open(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if err := r.Close(); err != nil {
					t.Fatal(err)
				}
				m, err := readManifest(dir)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(dir, m.Parts[0].Name)
				switch kind {
				case "source":
					cfg.Source = "simulator"
				case "missing_part":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				case "part_symlink":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(foreign, path); err != nil {
						t.Fatal(err)
					}
				case "traversal":
					m.Parts[0].Name = "../foreign.txt"
					if err := saveManifest(dir, m); err != nil {
						t.Fatal(err)
					}
				}
			}
			if r, err := Open(cfg); err == nil {
				r.Close()
				t.Fatal("unsafe storage accepted")
			}
			data, err := os.ReadFile(foreign)
			if err != nil || string(data) != "keep" {
				t.Fatalf("foreign file changed: %q %v", data, err)
			}
		})
	}
}

func TestQueueOverflowStickyAndOwnedData(t *testing.T) {
	t.Parallel()
	r := &Recorder{queue: make(chan Entry, queueSize), status: Status{State: "recording"}}
	data := json.RawMessage(`{"value":1}`)
	for range queueSize {
		if err := r.Submit(Entry{Kind: "sample", Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	data[0] = '!'
	if got := <-r.queue; got.Data[0] != '{' {
		t.Fatal("queue does not own data")
	}
	if err := r.Submit(Entry{Kind: "last"}); err != nil {
		t.Fatal(err)
	}
	err := r.Submit(Entry{Kind: "overflow"})
	if err == nil || !strings.Contains(err.Error(), "overflow") {
		t.Fatal(err)
	}
	if got := r.Submit(Entry{Kind: "after"}); got != err {
		t.Fatalf("failure was not sticky: %v", got)
	}
	if r.Status().State != "failed" {
		t.Fatal(r.Status())
	}
}

func TestWorkerFailureHasNoCleanFooter(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.disk.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(Entry{Kind: "sample"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Status().State == "failed" })
	if err := r.Close(); err == nil {
		t.Fatal("write failure lost")
	}
	if err := r.Close(); err == nil {
		t.Fatal("second close lost error")
	}
	for _, row := range readRows(t, dir) {
		if row.Kind == "session_end" {
			t.Fatal("false clean footer")
		}
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Parts[0].Closed || !m.Parts[0].Aborted {
		t.Fatal(m)
	}
}

func TestInvalidJSONFailsOnWorker(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Submit(Entry{Kind: "sample", Data: json.RawMessage(`broken`)}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return r.Status().State == "failed" })
	if err := r.Close(); err == nil {
		t.Fatal("invalid JSON not returned by Close")
	}
}

func TestPeriodicSyncFailure(t *testing.T) {
	t.Parallel()
	r, err := Open(DefaultConfig(realTemp(t), "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.disk.file.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for r.Status().State != "failed" {
		if time.Now().After(deadline) {
			t.Fatal("periodic fsync did not detect closed file")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.Close(); err == nil {
		t.Fatal("sync failure lost on close")
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestShortWrite(t *testing.T) {
	t.Parallel()
	if err := writeAll(shortWriter{}, []byte("line")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for worker")
		}
		time.Sleep(time.Millisecond)
	}
}
