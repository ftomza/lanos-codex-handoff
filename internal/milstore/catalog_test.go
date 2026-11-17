package milstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These synthetic archives catch accidental JSONL scans, writes, and loss of
// retained part state when the lightweight catalog groups rotated sessions.
func TestListCatalogRetainedPartStateAndReadOnly(t *testing.T) {
	t.Parallel()
	dir, m, _ := readerArchive(t)
	first := m.Parts[0]
	second := first
	second.Name = "mil-part-0000000000000002.jsonl"
	second.Closed, second.Aborted = false, true
	readerWriteRows(t, dir, second.Name, nil)
	m.Next, m.Truncated, m.Parts = 2, true, []part{first, second}
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	// Catalog must validate files but must not interpret damaged row content.
	if err := os.WriteFile(filepath.Join(dir, first.Name), []byte("invalid JSON\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := readerSnapshotArchive(t, dir)
	c, err := List(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 1 || c.Source != "live" || !c.RetentionTruncated || len(c.Sessions) != 1 {
		t.Fatalf("catalog=%+v", c)
	}
	s := c.Sessions[0]
	if s.ID != first.Session || !s.StartedAt.Equal(first.StartedAt) || s.Parts != 2 || s.Closed || !s.Aborted {
		t.Fatalf("session=%+v", s)
	}
	if !reflect.DeepEqual(before, readerSnapshotArchive(t, dir)) {
		t.Fatal("catalog modified archive contents, mtimes or listing")
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("catalog created lock: %v", err)
	}
	m.Parts = []part{first}
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	c, err = List(context.Background(), dir)
	if err != nil || len(c.Sessions) != 1 || !c.Sessions[0].Closed || c.Sessions[0].Aborted {
		t.Fatalf("catalog=%+v err=%v", c, err)
	}
}

func TestListCancellationAndFatalFiles(t *testing.T) {
	t.Parallel()
	dir, m, _ := readerArchive(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := List(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, m.Parts[0].Name)); err != nil {
		t.Fatal(err)
	}
	if _, err := List(context.Background(), dir); err == nil {
		t.Fatal("rotation/removal accepted")
	}
	missing := filepath.Join(dir, "missing")
	if _, err := List(context.Background(), missing); err == nil {
		t.Fatal("missing archive accepted")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("catalog created missing directory")
	}
}
