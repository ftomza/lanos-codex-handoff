package mil

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

// Synthetic accelerated time exercises the actual capture -> queue -> JSONL
// path. No adapter or vehicle is opened by this test.
func TestCompleteCaptureJSONLRoundTrip(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := milstore.Open(milstore.DefaultConfig(dir, telemetry.Live))
	if err != nil {
		t.Fatal(err)
	}
	c := NewCapture(w, nil, 3*time.Second)
	at := time.Now()
	for i := 0; i <= 100; i++ {
		now := at.Add(time.Duration(i) * time.Second)
		s := synthetic(now, i >= 70)
		if i == 71 {
			s.Poll = &telemetry.Poll{Command: "03", Status: "available", At: now, LatencyNS: 5000}
			s.ECUReadings = map[string]telemetry.ECUReading{"03": {Status: "available", CapturedAt: now, ReceivedAt: now, RawResponse: "43000000000000\r>", Detail: json.RawMessage(`{"codes":[]}`)}}
		}
		c.ObserveAt(now, s)
	}
	c.now = func() time.Time { return at.Add(101 * time.Second) }
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "mil-part-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	f, err := os.Open(files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	var sequence uint64
	counts := make(map[string]int)
	var window Window
	for scan.Scan() {
		var row struct {
			Version  int             `json:"version"`
			Sequence uint64          `json:"sequence"`
			Kind     string          `json:"kind"`
			Data     json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scan.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		if row.Version != 1 || row.Sequence != sequence+1 {
			t.Fatal("invalid saved sequence", row.Version, row.Sequence, sequence)
		}
		sequence = row.Sequence
		counts[row.Kind]++
		if row.Kind == "incident_end" {
			if err := json.Unmarshal(row.Data, &window); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if counts["session_start"] != 1 || counts["session_end"] != 1 || counts["diagnostic"] != 1 || counts["pre"] < 60 || counts["post"] < 29 || !window.PreWindowRetained || !window.PostWindowRetained {
		t.Fatal(counts, window)
	}
	if w.Status().State != "closed" || c.Status().State != "closed" {
		t.Fatal(c.Status())
	}
}
