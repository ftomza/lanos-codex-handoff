package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type memoryFile struct {
	bytes.Buffer
	writeErr, syncErr, closeErr error
	short                       bool
	syncs, closes               int
}

func (f *memoryFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.short {
		return len(p) / 2, nil
	}
	return f.Buffer.Write(p)
}
func (f *memoryFile) Sync() error  { f.syncs++; return f.syncErr }
func (f *memoryFile) Close() error { f.closes++; return f.closeErr }

func TestPrivateExclusiveJSONLines(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trip.jsonl")
	w, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if w.Path() != path {
		t.Fatal(w.Path())
	}
	stamp := time.Now().UTC()
	for _, kind := range []string{"recording_start", "command", "recording_end"} {
		if err := w.Write(Event{Type: kind, Timestamp: stamp, ElapsedNS: 98765432101234567, Segment: 1, Data: map[string]any{"error": "1HGCM82633A004352"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, true); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrote existing file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "1HGCM82633A004352") {
		t.Fatalf("read/redact: %s %v", data, err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	if len(lines) != 3 {
		t.Fatalf("lines=%d", len(lines))
	}
	for i, line := range lines {
		var e Event
		if err := json.Unmarshal(line, &e); err != nil || e.Sequence != uint64(i+1) || e.Version != 1 || e.ElapsedNS != 98765432101234567 || !e.Timestamp.Equal(stamp) {
			t.Fatalf("event=%+v err=%v", e, err)
		}
	}
}

func TestFileLimitPreservesFinalEvent(t *testing.T) {
	t.Parallel()
	f := &memoryFile{}
	w := newWriter(f, "memory", true)
	w.limit = finalReserve + 800
	e := Event{Type: "command", Timestamp: time.Now(), Data: strings.Repeat("x", 200)}
	count := 0
	for ; count < 20; count++ {
		if err := w.Write(e); err != nil {
			if !errors.Is(err, ErrSizeLimit) {
				t.Fatal(err)
			}
			break
		}
	}
	if count == 0 || count == 20 {
		t.Fatalf("limit not enforced: %d", count)
	}
	if err := w.Write(Event{Type: "recording_end", Timestamp: time.Now(), Data: "size_limit"}); err != nil {
		t.Fatal(err)
	}
	if int64(f.Len()) > w.limit || !strings.Contains(f.String(), "recording_end") {
		t.Fatal("missing reserved end or exceeded limit")
	}
}

func TestWriterFailuresAreStickyAndCloseErrorsPreserved(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"write", "short", "sync"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			cause := errors.New("disk failed")
			f := &memoryFile{closeErr: errors.New("close failed")}
			switch kind {
			case "write":
				f.writeErr = cause
			case "short":
				f.short = true
				cause = io.ErrShortWrite
			case "sync":
				f.syncErr = cause
			}
			w := newWriter(f, "memory", true)
			e := Event{Type: "command", Timestamp: time.Now()}
			if err := w.Write(e); !errors.Is(err, cause) {
				t.Fatalf("first error=%v", err)
			}
			if err := w.Write(e); !errors.Is(err, cause) {
				t.Fatalf("continued after failure: %v", err)
			}
			if err := w.Close(); !errors.Is(err, cause) || !errors.Is(err, f.closeErr) || f.closes != 1 {
				t.Fatalf("close: %v closes=%d", err, f.closes)
			}
			_ = w.Close()
			if f.closes != 1 {
				t.Fatal("closed twice")
			}
		})
	}
}

func TestCommandIsSyncedBeforeLongAdapterWait(t *testing.T) {
	t.Parallel()
	f := &memoryFile{}
	w := newWriter(f, "memory", true)
	if err := w.Write(Event{Type: "command", Timestamp: time.Now(), Data: "ATE0 OK"}); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 1 {
		t.Fatalf("unsynced command could wait 30s for protocol search: syncs=%d", f.syncs)
	}
}
