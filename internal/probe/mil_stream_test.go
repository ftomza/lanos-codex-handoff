package probe

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestStreamMILUsesSharedSessionAndInterleavesBoundedDiagnostics(t *testing.T) {
	t.Parallel()
	for _, noCore := range []bool{false, true} {
		t.Run(map[bool]string{false: "core", true: "no_core"}[noCore], func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["0100"] = "410080000000\r>"
			if !noCore {
				a.responses["0100"] = "4100A0100000\r>"
				a.responses["0103"] = "41030100\r>"
			}
			a.responses["0101"] = "41018007E1A1\r>"
			a.responses["07"] = "470133\r>"
			a.responses["0A"] = "7F0A11\r>"
			a.responses["020000"] = "42000040100000\r>"
			a.responses["020200"] = "4202000133\r>"
			a.responses["020C00"] = "420C001AF8\r>"
			r, clock := recordingRunner(t, a, 0)
			r.config.MILInterval = time.Second
			opens := 0
			r.open = func(context.Context) (elm327.Transport, error) { opens++; return a, nil }
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var polling []string
			ready := false
			started := *clock
			_, err := r.Stream(ctx, func(e record.Event) error {
				if e.Type == "capabilities" {
					ready = true
				}
				if ready && e.Type == "command" {
					polling = append(polling, e.Data.(RecordingCommand).Command)
				}
				if clock.Sub(started) > 40*time.Second {
					cancel()
				}
				return nil
			})
			if err != nil || opens != 1 || !a.closed {
				t.Fatalf("session: opens=%d closed=%v err=%v", opens, a.closed, err)
			}
			for _, command := range []string{"07", "0A", "020000", "020200", "020C00"} {
				if strings.Count(strings.Join(polling, ","), command) != 1 {
					t.Fatalf("command %s not exactly once: %v", command, polling)
				}
			}
			if slices.Contains(polling, "020300") || slices.Contains(polling, "04") || slices.Contains(polling, "08") {
				t.Fatalf("unadvertised/write command: %v", polling)
			}
			if count := strings.Count(strings.Join(polling, ","), "0101"); count < 30 {
				t.Fatalf("MIL starved: %d %v", count, polling)
			}
			if !noCore && !slices.Contains(polling, "0103") {
				t.Fatalf("support starved: %v", polling)
			}
		})
	}
}

func TestMILDisabledKeepsDefaultSchedulesAndFiniteRecord(t *testing.T) {
	t.Parallel()
	for _, finite := range []bool{false, true} {
		t.Run(map[bool]string{false: "stream_default", true: "record_opt_in_ignored"}[finite], func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["0101"] = "41018007E1A1\r>"
			r, clock := recordingRunner(t, a, 0)
			if finite {
				r.config.MILInterval = time.Second
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			start := *clock
			sink := func(record.Event) error {
				if clock.Sub(start) > 65*time.Second {
					cancel()
				}
				return nil
			}
			if finite {
				_, _ = r.Record(ctx, 70*time.Second, sink)
			} else {
				_, _ = r.Stream(ctx, sink)
			}
			for _, command := range []string{"07", "0A", "020000"} {
				if slices.Contains(a.commands, command) {
					t.Fatalf("default command changed: %v", a.commands)
				}
			}
			if count := strings.Count(strings.Join(a.commands, ","), "0101"); count != 2 {
				t.Fatalf("default MIL interval changed: %d", count)
			}
		})
	}
}

func TestStreamMILDisconnectStopsJobWithoutCleanupWrites(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0101"] = "41018007E1A1\r>"
	a.failCommand, a.disconnect = "07", true
	r, _ := recordingRunner(t, a, 0)
	r.config.MILInterval = time.Second
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) { opens++; return a, nil }
	summary, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "gap" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, io.EOF) || opens != 1 || !a.closed || summary.Gaps != 1 {
		t.Fatalf("disconnect: %+v %v opens=%d closed=%v", summary, err, opens, a.closed)
	}
	if a.commands[len(a.commands)-1] != "07" {
		t.Fatalf("commands after failed job: %v", a.commands)
	}
	for _, blocked := range []string{"04", "08", "ATH1", "020000", "0A"} {
		if slices.Contains(a.commands, blocked) {
			t.Fatalf("unexpected %s after disconnect: %v", blocked, a.commands)
		}
	}
}
