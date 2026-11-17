package milstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestStrictBounds(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"duplicate_key":      `{"version":1,"source":"simulator","source":"live","next":0,"retention_truncated":false,"parts":[]}`,
		"unknown_key":        `{"version":1,"source":"live","extra":true,"parts":[]}`,
		"trailing_data":      `{"version":1,"source":"live","parts":[]} {}`,
		"oversized":          strings.Repeat(" ", maxManifest+1),
		"exhausted_sequence": `{"version":1,"source":"live","next":18446744073709551615,"parts":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := realTemp(t)
			if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if r, err := Open(DefaultConfig(dir, "live")); err == nil {
				r.Close()
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}

func TestRejectExistingPartBeyondConfiguredLimit(t *testing.T) {
	t.Parallel()
	dir := realTemp(t)
	r, err := Open(DefaultConfig(dir, "live"))
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
	if err := os.Truncate(path, 3<<20); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(dir, "live")
	cfg.MaxBytes = 2 << 20
	if r, err := Open(cfg); err == nil {
		r.Close()
		t.Fatal("existing part exceeds configured byte bound")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{
		{}, {Dir: "x", Source: "LIVE", MaxBytes: 16 << 20, MaxFiles: 8},
		{Dir: "x", Source: "live", MaxBytes: 1 << 20, MaxFiles: 8},
		{Dir: "x", Source: "live", MaxBytes: 65 << 20, MaxFiles: 8},
		{Dir: "x", Source: "live", MaxBytes: 16 << 20, MaxFiles: 33},
		{Dir: "x", Source: "live", MaxBytes: 16 << 20, MaxFiles: 0},
	} {
		t.Run(fmt.Sprint(cfg), func(t *testing.T) {
			t.Parallel()
			if _, err := Open(cfg); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
