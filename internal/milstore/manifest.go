package milstore

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const manifestName = ".mil-manifest.json"
const lockName = ".mil-writer.lock"
const maxManifest = 64 << 10

type part struct {
	Name      string    `json:"name"`
	Session   string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	Closed    bool      `json:"closed"`
	Aborted   bool      `json:"aborted,omitempty"`
}

type manifest struct {
	Version   int    `json:"version"`
	Source    string `json:"source"`
	Next      uint64 `json:"next"`
	Truncated bool   `json:"retention_truncated"`
	Parts     []part `json:"parts"`
}

func partNumber(name string) (uint64, bool) {
	if len(name) != 31 || !strings.HasPrefix(name, "mil-part-") || !strings.HasSuffix(name, ".jsonl") || strings.ToLower(name) != name {
		return 0, false
	}
	n, err := strconv.ParseUint(name[9:25], 16, 64)
	return n, err == nil && n != 0
}

func validSession(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 32 && strings.ToLower(s) == s && err == nil
}

func regular(path string) (os.FileInfo, error) {
	i, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !i.Mode().IsRegular() {
		return nil, fmt.Errorf("mil managed file must be regular: %s", filepath.Base(path))
	}
	if i.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("mil managed file is not private: %s", filepath.Base(path))
	}
	return i, nil
}

func readManifest(dir string) (manifest, error) {
	var m manifest
	path := filepath.Join(dir, manifestName)
	info, err := regular(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, err
	}
	if err != nil {
		return m, err
	}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	actual, statErr := f.Stat()
	if statErr != nil || !os.SameFile(info, actual) {
		return m, errors.Join(fmt.Errorf("mil manifest changed while opening"), statErr, f.Close())
	}
	data, err := io.ReadAll(io.LimitReader(f, maxManifest+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return m, err
	}
	if len(data) > maxManifest {
		return m, fmt.Errorf("mil manifest exceeds limit")
	}
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return m, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("invalid MIL manifest: %w", err)
	}
	var extra any
	if !errors.Is(d.Decode(&extra), io.EOF) {
		return m, fmt.Errorf("extra MIL manifest data")
	}
	return m, validateManifest(dir, m)
}

// encoding/json otherwise accepts duplicate object keys using the last value.
func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return fmt.Errorf("mil manifest nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return fmt.Errorf("invalid MIL manifest delimiter")
	}
	keys := make(map[string]bool)
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || keys[name] {
				return fmt.Errorf("duplicate or invalid MIL manifest key")
			}
			keys[name] = true
		}
		if err := uniqueJSON(d, depth+1); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func validateManifest(dir string, m manifest) error {
	if m.Version != Version || m.Source != "live" && m.Source != "simulator" || len(m.Parts) > 32 || m.Next == ^uint64(0) {
		return fmt.Errorf("invalid MIL manifest metadata")
	}
	var previous uint64
	for _, p := range m.Parts {
		n, ok := partNumber(p.Name)
		if !ok || n <= previous || n > m.Next || !validSession(p.Session) || p.StartedAt.IsZero() || p.Aborted && p.Closed {
			return fmt.Errorf("invalid MIL manifest part")
		}
		if _, err := regular(filepath.Join(dir, p.Name)); err != nil {
			return err
		}
		previous = n
	}
	return nil
}

func rejectOrphans(dir string, m manifest) error {
	owned := make(map[string]bool, len(m.Parts))
	for _, p := range m.Parts {
		owned[p.Name] = true
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, ok := partNumber(e.Name()); ok && !owned[e.Name()] {
			return fmt.Errorf("orphan MIL part %s preserved; inspect it and the manifest before retrying", e.Name())
		}
	}
	return nil
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func saveManifest(dir string, m manifest) (err error) {
	path := filepath.Join(dir, manifestName)
	if _, e := regular(path); e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	f, err := os.CreateTemp(dir, ".mil-manifest-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !errors.Is(e, os.ErrNotExist) {
			err = errors.Join(err, e)
		}
	}()
	data, err := json.Marshal(m)
	if err == nil {
		err = writeAll(f, append(data, '\n'))
	}
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDir(dir)
}

func privateDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	// Reject symlink components, including ancestors of newly-created directories.
	current := string(filepath.Separator)
	for _, name := range strings.Split(strings.TrimPrefix(abs, current), string(filepath.Separator)) {
		current = filepath.Join(current, name)
		info, e := os.Lstat(current)
		if errors.Is(e, os.ErrNotExist) {
			if e = os.Mkdir(current, 0700); e != nil {
				return e
			}
			continue
		}
		if e != nil {
			return e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("mil directory must have real directory components")
		}
	}
	return nil
}
