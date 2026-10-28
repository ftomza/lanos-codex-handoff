package fuel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestFileStateLockPrivateDurabilityAndRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "private", "trip.json")
	c, err := Open(DefaultConfig(), telemetry.Simulator, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(DefaultConfig(), telemetry.Simulator, path, false); err == nil {
		t.Fatal("concurrent writer opened")
	}
	now := time.Now()
	c.Observe(now, drive(now, 3.6, 36))
	c.Observe(now.Add(time.Second), drive(now.Add(time.Second), 3.6, 36))
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("state not private", err)
	}
	c, err = Open(DefaultConfig(), telemetry.Simulator, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if v := c.View(time.Now()); v.Liters != .001 || !v.Incomplete {
		t.Fatalf("restart lost state: %+v", v)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatal("own lock not removed")
	}
}
func TestCorruptStateAndConfigMismatchNeverResetSilently(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"{", `{"version":99}`, `{"unexpected":true}`, `{} {}`, strings.Repeat("x", 65537)} {
		t.Run(raw[:min(20, len(raw))], func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "state.json")
			if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(DefaultConfig(), telemetry.Simulator, path, false); err == nil {
				t.Fatal("corruption accepted")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != raw {
				t.Fatal("corrupt state overwritten")
			}
			if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
				t.Fatal("failed open leaked lock")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "state.json")
	c, err := Open(DefaultConfig(), telemetry.Simulator, path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(DefaultConfig(), telemetry.Live, path, false); err == nil {
		t.Fatal("mixed live/sim state")
	}
	c, err = Open(DefaultConfig(), telemetry.Live, path, true)
	if err != nil {
		t.Fatal("explicit startup reset failed", err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestAtomicRenameFailureKeepsCommittedState(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trip.json")
	s, err := openFileStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	state := State{Version: 1, Source: telemetry.Simulator, Fingerprint: DefaultConfig().fingerprint(), Revision: 1, ResetAt: now, SavedAt: now, Factor: 1}
	if err := s.Save(state); err == nil {
		t.Fatal("rename into directory succeeded")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".fuel-") {
			t.Fatal("temporary file leaked")
		}
	}
}
