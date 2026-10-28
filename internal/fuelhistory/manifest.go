package fuelhistory

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

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type part struct {
	Name      string    `json:"name"`
	Session   string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	Closed    bool      `json:"closed"`
}
type manifest struct {
	Version   int    `json:"version"`
	Source    string `json:"source"`
	Next      uint64 `json:"next"`
	Truncated bool   `json:"retention_truncated"`
	Parts     []part `json:"parts"`
}

func validSession(s string) bool {
	_, err := hex.DecodeString(s)
	return len(s) == 32 && err == nil && strings.ToLower(s) == s
}
func validPart(s string) bool {
	if len(s) != 27 || !strings.HasPrefix(s, "part-") || !strings.HasSuffix(s, ".jsonl") {
		return false
	}
	_, err := hex.DecodeString(s[5:21])
	return err == nil && strings.ToLower(s) == s
}
func regular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("history file is not regular")
	}
	return nil
}
func loadManifest(dir string) (manifest, error) {
	var m manifest
	path := filepath.Join(dir, "manifest.json")
	if err := regular(path); err != nil {
		return m, err
	}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxLine+1))
	err = errors.Join(err, f.Close())
	if err != nil {
		return m, err
	}
	if len(data) > MaxLine {
		return m, fmt.Errorf("history manifest exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, err
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return m, fmt.Errorf("extra manifest data")
	}
	if m.Version != Version || m.Source != telemetry.Live && m.Source != telemetry.Simulator || len(m.Parts) > 32 || m.Next == ^uint64(0) {
		return m, fmt.Errorf("invalid history manifest metadata")
	}
	seen := make(map[string]bool, len(m.Parts))
	var previous uint64
	for _, p := range m.Parts {
		if !validPart(p.Name) || !validSession(p.Session) || p.StartedAt.IsZero() || seen[p.Name] {
			return m, fmt.Errorf("invalid history manifest part")
		}
		number, err := strconv.ParseUint(p.Name[5:21], 16, 64)
		if err != nil || number <= previous || number > m.Next {
			return m, fmt.Errorf("invalid history part sequence")
		}
		previous = number
		if err := regular(filepath.Join(dir, p.Name)); err != nil {
			return m, err
		}
		seen[p.Name] = true
	}
	return m, nil
}
func saveManifest(dir string, m manifest) (err error) {
	f, err := os.CreateTemp(dir, ".manifest-*")
	if err != nil {
		return err
	}
	defer func() {
		if e := os.Remove(f.Name()); !errors.Is(e, os.ErrNotExist) {
			err = errors.Join(err, e)
		}
	}()
	err = json.NewEncoder(f).Encode(m)
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, "manifest.json")); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
func List(dir string) ([]Session, error) {
	m, err := loadManifest(dir)
	if err != nil {
		return nil, err
	}
	sessions := make([]Session, 0, len(m.Parts))
	index := make(map[string]int, len(m.Parts))
	for _, p := range m.Parts {
		i, ok := index[p.Session]
		if !ok {
			i = len(sessions)
			index[p.Session] = i
			sessions = append(sessions, Session{ID: p.Session, Source: m.Source, StartedAt: p.StartedAt, Closed: true, RetainedOnly: m.Truncated && i == 0})
		}
		sessions[i].Parts++
		sessions[i].Closed = sessions[i].Closed && p.Closed
	}
	return sessions, nil
}
