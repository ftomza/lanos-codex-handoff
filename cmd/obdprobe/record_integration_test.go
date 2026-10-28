package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

// Replays previously saved physical responses. Missing responses are explicitly
// synthetic NO DATA; this test never starts CoreBluetooth or accesses a device.
type recordedWire struct {
	responses map[string]string
	commands  []string
	pending   []byte
	closed    bool
}

func (w *recordedWire) Read(ctx context.Context, b []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(w.pending) == 0 {
		return 0, io.EOF
	}
	n := copy(b, w.pending)
	w.pending = w.pending[n:]
	return n, nil
}
func (w *recordedWire) Write(ctx context.Context, b []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	command := strings.TrimSuffix(string(b), "\r")
	w.commands = append(w.commands, command)
	raw, ok := w.responses[command]
	if !ok {
		raw = "NO DATA\r>"
	}
	w.pending = append(w.pending, raw...)
	return len(b), nil
}
func (w *recordedWire) Close() error { w.closed = true; return nil }

func TestRecordCLIThroughSharedELMAndPrivateWriter(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../internal/elm327/testdata/vgate-ble-2026-10-24.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observations []struct{ Command, Raw string }
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	wire := &recordedWire{responses: make(map[string]string)}
	for _, observation := range fixture.Observations {
		wire.responses[observation.Command] = observation.Raw
	}
	path := filepath.Join(t.TempDir(), "trip.jsonl")
	var out, stderr bytes.Buffer
	deps := dependencies{record: func(ctx context.Context, cfg probe.Config, duration time.Duration, sink func(record.Event) error) (probe.RecordingSummary, error) {
		runner, err := probe.New(cfg, func(context.Context) (elm327.Transport, error) { return wire, nil })
		if err != nil {
			return probe.RecordingSummary{}, err
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		return runner.Record(ctx, duration, func(e record.Event) error {
			err := sink(e)
			if e.Type == "capabilities" {
				cancel()
			}
			return err
		})
	}}
	if err := run(t.Context(), []string{"--transport=ble", "--record", "--output=" + path}, &out, &stderr, deps); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte{'\n'})
	var foundRPM, foundEmptyDTC bool
	for i, line := range lines {
		var e struct {
			Type      string
			Sequence  uint64
			ElapsedNS int64 `json:"elapsed_ns"`
			Data      json.RawMessage
		}
		if err := json.Unmarshal(line, &e); err != nil || e.Sequence != uint64(i+1) || e.ElapsedNS < 0 {
			t.Fatalf("event %s err=%v", line, err)
		}
		if e.Type == "command" {
			var c probe.RecordingCommand
			if err := json.Unmarshal(e.Data, &c); err != nil {
				t.Fatal(err)
			}
			if c.Command == "010C" && c.Result != nil && c.Result.Value != nil && *c.Result.Value == 889.25 {
				foundRPM = true
			}
			if c.Command == "03" && c.Result != nil && c.Result.Status == "available" && bytes.Contains(e.Data, []byte(`"codes":[]`)) {
				foundEmptyDTC = true
			}
		}
	}
	if !foundRPM || !foundEmptyDTC || !wire.closed || len(wire.commands) != 27 || slices.Contains(wire.commands, "0110") || !strings.Contains(out.String(), "canceled") {
		t.Fatalf("end-to-end evidence: rpm=%t DTC=%t closed=%t commands=%v out=%s", foundRPM, foundEmptyDTC, wire.closed, wire.commands, out.String())
	}
}
