package milstore

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecorderPrivateJSONL(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(realTemp(t), "private")
	r, err := Open(DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{"raw":"WVWZZZ1JZXW000001","value":12}`)
	if err := r.Submit(Entry{Kind: "incident", At: time.Now(), Incident: "one", Data: data}); err != nil {
		t.Fatal(err)
	}
	data[0] = '!'
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	rows := readRows(t, dir)
	if len(rows) != 3 {
		t.Fatalf("rows=%d", len(rows))
	}
	for i, row := range rows {
		if row.Version != 1 || row.Sequence != uint64(i+1) || row.Source != "live" || row.Session == "" {
			t.Fatalf("row=%+v", row)
		}
	}
	if rows[0].Kind != "session_start" || rows[2].Kind != "session_end" {
		t.Fatal(rows)
	}
	if strings.Contains(string(rows[1].Data), "WVWZZZ1JZXW000001") {
		t.Fatal("VIN was not redacted")
	}
	if r.Status().State != "closed" {
		t.Fatal(r.Status())
	}
	files, _ := filepath.Glob(filepath.Join(dir, "mil-part-*.jsonl"))
	info, _ := os.Stat(files[0])
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	info, _ = os.Stat(dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal(info.Mode())
	}
}

func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func readRows(t *testing.T, dir string) []diskEntry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "mil-part-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []diskEntry
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s := bufio.NewScanner(f)
		s.Buffer(make([]byte, 4096), MaxLine)
		for s.Scan() {
			var row diskEntry
			if err := json.Unmarshal(s.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)
		}
		if err := s.Err(); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return rows
}
