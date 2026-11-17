package milstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Reader fixtures are synthetic archives produced by the real recorder.
func readerArchive(t *testing.T) (string, manifest, []diskEntry) {
	t.Helper()
	dir := filepath.Join(realTemp(t), "archive")
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	data, _ := json.Marshal(map[string]any{"id": 1, "kind": "MIL_INITIAL_OFF", "state": "OFF", "at": at, "generation": 1})
	if err := r.Submit(Entry{Kind: "event", At: at, Data: data}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, m, readRows(t, dir)
}

func readerWriteRows(t *testing.T, dir, name string, rows []diskEntry) {
	t.Helper()
	var data []byte
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		data = append(append(data, b...), '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func readerCollect(t *testing.T, dir string) ([]ReadEntry, []ReadIssue) {
	t.Helper()
	var entries []ReadEntry
	issues, err := Read(context.Background(), dir, ReadOptions{}, func(e ReadEntry) error { entries = append(entries, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return entries, issues
}

func readerHasIssue(issues []ReadIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestReadEntryJSONUsesArchiveEnvelope(t *testing.T) {
	t.Parallel()
	entry := Entry{Kind: "event", At: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC), Data: json.RawMessage(`{"synthetic":true}`)}
	row := ReadEntry{Version: Version, Session: strings.Repeat("a", 32), Source: "live", Sequence: 7, ElapsedNS: 1234, Entry: entry, File: "private.jsonl", Line: 42, Boundary: true}
	got, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(diskEntry{Version: Version, Session: row.Session, Source: row.Source, Sequence: row.Sequence, ElapsedNS: row.ElapsedNS, Entry: entry})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("read entry JSON=%s; want archive envelope=%s", got, want)
	}
}

type readerFileState struct {
	Hash     [32]byte
	Modified time.Time
}

type readerArchiveState struct {
	Names []string
	Files map[string]readerFileState
}

func readerSnapshotArchive(t *testing.T, dir string) readerArchiveState {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := readerArchiveState{Files: make(map[string]readerFileState, len(entries))}
	for _, entry := range entries {
		state.Names = append(state.Names, entry.Name())
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		state.Files[entry.Name()] = readerFileState{Hash: sha256.Sum256(data), Modified: info.ModTime()}
	}
	return state
}

func TestReadRecorderArchive(t *testing.T) {
	t.Parallel()
	dir, m, rows := readerArchive(t)
	before := readerSnapshotArchive(t, dir)
	entries, issues := readerCollect(t, dir)
	if len(issues) != 0 || len(entries) != 3 {
		t.Fatalf("entries=%d issues=%+v", len(entries), issues)
	}
	for i, e := range entries {
		if e.Version != Version || e.Session != rows[i].Session || e.Source != "live" || e.Sequence != uint64(i+1) || e.ElapsedNS != rows[i].ElapsedNS || e.File != m.Parts[0].Name || e.Line != i+1 || e.Boundary != (i == 0) {
			t.Fatalf("entry=%+v", e)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reader created lock: %v", err)
	}
	_, err := Read(context.Background(), dir, ReadOptions{Session: strings.Repeat("f", 32)}, func(ReadEntry) error { return nil })
	if err == nil {
		t.Fatal("unknown session accepted")
	}
	after := readerSnapshotArchive(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("read changed archive listing, hashes or modification times: before=%+v after=%+v", before, after)
	}
}

func TestReadRotationAndRetention(t *testing.T) {
	t.Parallel()
	dir, m, rows := readerArchive(t)
	first := m.Parts[0]
	second := first
	second.Name = "mil-part-0000000000000002.jsonl"
	m.Next = 2
	m.Parts = []part{first, second}
	readerWriteRows(t, dir, first.Name, rows[:2])
	readerWriteRows(t, dir, second.Name, rows[2:])
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	entries, issues := readerCollect(t, dir)
	if len(issues) != 0 || len(entries) != 3 || entries[2].Boundary {
		t.Fatalf("entries=%+v issues=%+v", entries, issues)
	}
	m.Parts = m.Parts[1:]
	m.Truncated = true
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	entries, issues = readerCollect(t, dir)
	if len(entries) != 1 || !entries[0].Boundary || !readerHasIssue(issues, "retention_truncated") || !readerHasIssue(issues, "incomplete") {
		t.Fatalf("entries=%+v issues=%+v", entries, issues)
	}
}

func TestReadDamagedRowsNeverReachSink(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*diskEntry)
	}{
		{"version", func(e *diskEntry) { e.Version = 2 }},
		{"session", func(e *diskEntry) { e.Session = strings.Repeat("f", 32) }},
		{"source", func(e *diskEntry) { e.Source = "simulator" }},
		{"kind", func(e *diskEntry) { e.Kind = "surprise" }},
		{"payload", func(e *diskEntry) { e.Data = json.RawMessage(`{"id":"bad"}`) }},
		{"null_payload", func(e *diskEntry) { e.Data = json.RawMessage(`null`) }},
		{"zero_at", func(e *diskEntry) { e.At = time.Time{} }},
		{"negative_elapsed", func(e *diskEntry) { e.ElapsedNS = -1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, m, rows := readerArchive(t)
			tc.mutate(&rows[1])
			readerWriteRows(t, dir, m.Parts[0].Name, rows)
			entries, issues := readerCollect(t, dir)
			if len(entries) != 2 || !entries[1].Boundary || !readerHasIssue(issues, "invalid_row") {
				t.Fatalf("entries=%+v issues=%+v", entries, issues)
			}
		})
	}
}

func TestReadSequenceElapsedAndFooter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, code string
		mutate     func([]diskEntry) []diskEntry
	}{
		{"sequence", "sequence_gap", func(r []diskEntry) []diskEntry { r[1].Sequence++; r[2].Sequence++; return r }},
		{"elapsed", "elapsed_regression", func(r []diskEntry) []diskEntry { r[2].ElapsedNS = 0; return r }},
		{"missing_start", "incomplete", func(r []diskEntry) []diskEntry { return r[1:] }},
		{"missing_end", "incomplete", func(r []diskEntry) []diskEntry { return r[:2] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, m, rows := readerArchive(t)
			readerWriteRows(t, dir, m.Parts[0].Name, tc.mutate(rows))
			_, issues := readerCollect(t, dir)
			if !readerHasIssue(issues, tc.code) {
				t.Fatal(issues)
			}
		})
	}
}

func TestReadPartialTailAndBoundedIssues(t *testing.T) {
	t.Parallel()
	dir, m, _ := readerArchive(t)
	path := filepath.Join(dir, m.Parts[0].Name)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(strings.Repeat("broken\n", 140) + `{"partial":`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	_, issues := readerCollect(t, dir)
	if len(issues) != 128 || !readerHasIssue(issues, "issues_truncated") {
		t.Fatalf("issues=%d %+v", len(issues), issues)
	}
}

func TestReadLimitsAndIncompleteParts(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"large_line", "large_part", "active", "aborted"} {
		t.Run(name, func(t *testing.T) {
			dir, m, _ := readerArchive(t)
			code := "incomplete"
			switch name {
			case "large_line":
				code = "line_too_large"
				if err := os.WriteFile(filepath.Join(dir, m.Parts[0].Name), []byte(strings.Repeat("x", MaxLine+1)+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "large_part":
				code = "part_too_large"
				if err := os.Truncate(filepath.Join(dir, m.Parts[0].Name), (64<<20)+1); err != nil {
					t.Fatal(err)
				}
			default:
				m.Parts[0].Closed = false
				m.Parts[0].Aborted = name == "aborted"
				if err := saveManifest(dir, m); err != nil {
					t.Fatal(err)
				}
			}
			_, issues := readerCollect(t, dir)
			if !readerHasIssue(issues, code) {
				t.Fatal(issues)
			}
		})
	}
}

func TestReadCancelSinkAndPrivateFiles(t *testing.T) {
	t.Parallel()
	dir, m, _ := readerArchive(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Read(ctx, dir, ReadOptions{}, func(ReadEntry) error { t.Fatal("sink called after cancel"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	want := errors.New("sink failed")
	if _, err := Read(context.Background(), dir, ReadOptions{}, func(ReadEntry) error { return want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	if _, err := Read(ctx, dir, ReadOptions{}, func(ReadEntry) error { calls++; cancel(); return nil }); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	if err := os.Chmod(filepath.Join(dir, m.Parts[0].Name), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), dir, ReadOptions{}, func(ReadEntry) error { return nil }); err == nil {
		t.Fatal("public file accepted")
	}
}

func TestReadRequiresFooterInFinalSessionPart(t *testing.T) {
	t.Parallel()
	dir, m, _ := readerArchive(t)
	p := m.Parts[0]
	p.Name = "mil-part-0000000000000002.jsonl"
	m.Next = 2
	m.Parts = append(m.Parts, p)
	readerWriteRows(t, dir, p.Name, nil)
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	_, issues := readerCollect(t, dir)
	if !readerHasIssue(issues, "incomplete") {
		t.Fatalf("footer in earlier part hid incomplete final part: %+v", issues)
	}
}

func TestReadBackwardPreObservationPreservesContinuity(t *testing.T) {
	t.Parallel()
	dir, m, rows := readerArchive(t)
	at := rows[0].At.Add(-time.Minute)
	data, _ := json.Marshal(map[string]any{"at": at, "effective_health": "connected", "snapshot": map[string]any{"source": "live"}})
	rows[1].Kind, rows[1].Incident, rows[1].At, rows[1].Data = "pre", "synthetic", at, data
	readerWriteRows(t, dir, m.Parts[0].Name, rows)
	entries, issues := readerCollect(t, dir)
	if len(issues) != 0 || len(entries) != 3 || entries[1].Boundary {
		t.Fatalf("entries=%+v issues=%+v", entries, issues)
	}
}

func TestReadRejectsUnsafeArchiveWithoutMutation(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"symlink_part", "symlink_manifest", "symlink_directory", "path_traversal", "manifest_version", "duplicate_manifest_key", "duplicate_row_key", "trailing_row_json", "missing_directory"} {
		t.Run(name, func(t *testing.T) {
			dir, m, _ := readerArchive(t)
			input := dir
			rowIssue := false
			switch name {
			case "symlink_part", "symlink_manifest":
				path := filepath.Join(dir, m.Parts[0].Name)
				if name == "symlink_manifest" {
					path = filepath.Join(dir, manifestName)
				}
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "symlink_directory":
				input = filepath.Join(realTemp(t), "alias")
				if err := os.Symlink(dir, input); err != nil {
					t.Fatal(err)
				}
			case "path_traversal":
				m.Parts[0].Name = "../outside.jsonl"
				if err := saveManifest(dir, m); err != nil {
					t.Fatal(err)
				}
			case "manifest_version":
				m.Version = 2
				if err := saveManifest(dir, m); err != nil {
					t.Fatal(err)
				}
			case "duplicate_manifest_key":
				if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(`{"version":1,"version":1}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate_row_key", "trailing_row_json":
				rowIssue = true
				path := filepath.Join(dir, m.Parts[0].Name)
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if name == "duplicate_row_key" {
					data = bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1)
				} else {
					data = bytes.Replace(data, []byte("\n"), []byte(" {}\n"), 1)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_directory":
				input = filepath.Join(dir, "missing")
			}
			issues, err := Read(context.Background(), input, ReadOptions{}, func(ReadEntry) error { return nil })
			if rowIssue {
				if err != nil || !readerHasIssue(issues, "invalid_row") {
					t.Fatalf("issues=%+v err=%v", issues, err)
				}
			} else if err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, lockName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("reader mutated lock: %v", err)
			}
			if name == "missing_directory" {
				if _, err := os.Stat(input); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("reader created directory")
				}
			}
		})
	}
}

func TestReadRejectsMissingAndNullRequiredMetadata(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"version", "session_id", "source", "sequence", "elapsed_ns", "kind", "at"} {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/null=%t", field, null), func(t *testing.T) {
				dir, m, rows := readerArchive(t)
				var row map[string]json.RawMessage
				data, _ := json.Marshal(rows[0])
				if err := json.Unmarshal(data, &row); err != nil {
					t.Fatal(err)
				}
				if null {
					row[field] = json.RawMessage("null")
				} else {
					delete(row, field)
				}
				data, _ = json.Marshal(row)
				path := filepath.Join(dir, m.Parts[0].Name)
				if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
					t.Fatal(err)
				}
				entries, issues := readerCollect(t, dir)
				if len(entries) != 0 || !readerHasIssue(issues, "invalid_row") {
					t.Fatalf("entries=%+v issues=%+v", entries, issues)
				}
			})
		}
	}
}

func TestReadSelectedSessionAndNewSessionBoundary(t *testing.T) {
	t.Parallel()
	dir, _, _ := readerArchive(t)
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	session := r.Status().Session
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	all, issues := readerCollect(t, dir)
	if len(issues) != 0 || len(all) != 5 || !all[3].Boundary {
		t.Fatalf("entries=%+v issues=%+v", all, issues)
	}
	var selected []ReadEntry
	issues, err = Read(context.Background(), dir, ReadOptions{Session: session}, func(e ReadEntry) error { selected = append(selected, e); return nil })
	if err != nil || len(issues) != 0 || len(selected) != 2 || selected[0].Session != session || !selected[0].Boundary {
		t.Fatalf("entries=%+v issues=%+v err=%v", selected, issues, err)
	}
}

func TestReadSkipsCorruptAndPartialRows(t *testing.T) {
	t.Parallel()
	dir, m, rows := readerArchive(t)
	path := filepath.Join(dir, m.Parts[0].Name)
	start, _ := json.Marshal(rows[0])
	end, _ := json.Marshal(rows[2])
	data := append(start, '\n')
	data = append(data, []byte("broken JSON\n")...)
	data = append(data, end...)
	// A valid JSON object without its final newline is still an incomplete row.
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	entries, issues := readerCollect(t, dir)
	if len(entries) != 1 || !readerHasIssue(issues, "invalid_row") || !readerHasIssue(issues, "incomplete") {
		t.Fatalf("entries=%+v issues=%+v", entries, issues)
	}
}

func TestReadDuplicateSequenceCannotBecomeEvidence(t *testing.T) {
	t.Parallel()
	dir, m, rows := readerArchive(t)
	rows = append(rows[:2], append([]diskEntry{rows[1]}, rows[2:]...)...)
	readerWriteRows(t, dir, m.Parts[0].Name, rows)
	entries, issues := readerCollect(t, dir)
	if len(entries) != 3 || !entries[2].Boundary || !readerHasIssue(issues, "sequence_gap") {
		t.Fatalf("entries=%+v issues=%+v", entries, issues)
	}
}
