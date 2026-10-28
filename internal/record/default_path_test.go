package record

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultPathsArePrivateAndUnique(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDefaultPathWorker$")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"LANOS_RECORD_PATH_HELPER=1"}
	if data, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("default path helper: %v %s", err, data)
	}
	entries, err := os.ReadDir(filepath.Join(cmd.Dir, "diagnostics"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("files: %v %v", entries, err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !strings.HasPrefix(entry.Name(), "trip-") || !strings.HasSuffix(entry.Name(), ".jsonl") || info.Mode().Perm() != 0600 {
			t.Fatalf("default file: %v %v", info, err)
		}
	}
}

func TestDefaultPathWorker(t *testing.T) {
	t.Parallel()
	if os.Getenv("LANOS_RECORD_PATH_HELPER") != "1" {
		t.Skip("subprocess-only isolated working directory")
	}
	for range 2 {
		w, err := Open("", true)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Write(Event{Type: "recording_start", Timestamp: time.Now()}); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
