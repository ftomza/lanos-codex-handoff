package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestRecordFlags(t *testing.T) {
	t.Parallel()
	o, err := parseFlags([]string{"--transport=ble", "--record"}, &bytes.Buffer{})
	if err != nil || !o.record || o.duration != 4*time.Hour || o.config.Retries != 2 {
		t.Fatalf("defaults: %+v %v", o, err)
	}
	for _, args := range [][]string{
		{"--record", "--duration=0s"}, {"--record", "--duration=25h"}, {"--duration=1h"},
		{"--record", "--json"}, {"--record", "--list-ports"}, {"--record", "--scan-supported"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			if _, err := parseFlags(args, &bytes.Buffer{}); err == nil {
				t.Fatalf("accepted: %v", args)
			}
		})
	}
}

func TestRecordCLIStreamsPrivatePartialFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trip.jsonl")
	var out, stderr bytes.Buffer
	cause := errors.New("link lost")
	calls := 0
	deps := dependencies{record: func(ctx context.Context, cfg probe.Config, duration time.Duration, sink func(record.Event) error) (probe.RecordingSummary, error) {
		calls++
		if cfg.Transport != "ble" || duration != time.Hour {
			t.Fatalf("config: %+v duration=%s", cfg, duration)
		}
		for _, kind := range []string{"recording_start", "command", "gap", "recording_end"} {
			if err := sink(record.Event{Type: kind, Timestamp: time.Now(), Data: map[string]string{"error": "unsolicited 1HGCM82633A004352"}}); err != nil {
				return probe.RecordingSummary{}, err
			}
		}
		return probe.RecordingSummary{Reason: "failed", Commands: 1, Gaps: 1}, cause
	}}
	args := []string{"--transport=ble", "--record", "--duration=1h", "--output=" + path}
	err := run(t.Context(), args, &out, &stderr, deps)
	if !errors.Is(err, cause) || calls != 1 || !strings.Contains(out.String(), path) || !strings.Contains(out.String(), "failed") {
		t.Fatalf("CLI: %v %s %s", err, out.String(), stderr.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "1HGCM82633A004352") {
		t.Fatalf("data=%s err=%v", data, err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		if !json.Valid(line) {
			t.Fatal(string(line))
		}
	}
	if err := run(t.Context(), args, &out, &stderr, deps); !errors.Is(err, os.ErrExist) || calls != 1 {
		t.Fatalf("overwrite/hardware: %v calls=%d", err, calls)
	}
}

func TestRecordTerminalFailurePreventsHardware(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "trip.jsonl")
	called := false
	err := run(t.Context(), []string{"--transport=ble", "--record", "--output=" + path}, brokenWriter{}, &bytes.Buffer{}, dependencies{record: func(context.Context, probe.Config, time.Duration, func(record.Event) error) (probe.RecordingSummary, error) {
		called = true
		return probe.RecordingSummary{}, nil
	}})
	if err == nil || called {
		t.Fatalf("continued after terminal failure: %v called=%t", err, called)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("terminal closed") }
