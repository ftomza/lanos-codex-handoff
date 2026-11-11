package probe

import (
	"context"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestStreamStandbyReinitializesUnresponsiveECUSession(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	bitmap := a.responses["0100"]
	a.responses["0100"] = "NO DATA\r>"
	r, clock := recordingRunner(t, a, 0)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	opens, waits, initialized, ready := 0, 0, 0, 0
	r.open = func(context.Context) (elm327.Transport, error) { opens++; return a, nil }
	r.wait = func(ctx context.Context, delay time.Duration) error {
		if delay != 10*time.Second {
			t.Errorf("standby delay: %s", delay)
		}
		*clock = clock.Add(delay)
		waits++
		if waits > 4 {
			cancel()
		}
		return ctx.Err()
	}
	summary, err := r.Stream(ctx, func(event record.Event) error {
		if event.Type == "command" {
			command := event.Data.(RecordingCommand).Command
			if command == "ATSP0" {
				initialized++
				if initialized == 2 {
					a.responses["0100"] = bitmap
				}
			}
			if command == "03" && initialized < 2 {
				t.Error("DTC scan before ECU responded")
			}
		}
		if event.Type == "capabilities" {
			ready++
			cancel()
		}
		return nil
	})
	if err != nil || ready != 1 || initialized != 2 || waits != 3 || opens != 1 || summary.Segments != 2 || !a.closed {
		t.Fatalf("ECU session did not reinitialize/resume: err=%v ready=%d init=%d waits=%d opens=%d summary=%+v closed=%t", err, ready, initialized, waits, opens, summary, a.closed)
	}
}

func TestStreamStandbyReinitializationStaysSparseAndCancels(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0100"] = "NO DATA\r>"
	r, clock := recordingRunner(t, a, 0)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	waits, recoveries, commandsAtCancel := 0, 0, 0
	r.wait = func(ctx context.Context, delay time.Duration) error {
		if delay != 10*time.Second {
			t.Errorf("unbounded standby retry: %s", delay)
		}
		*clock = clock.Add(delay)
		waits++
		if waits == 9 {
			commandsAtCancel = len(a.commands)
			cancel()
		}
		return ctx.Err()
	}
	summary, err := r.Stream(ctx, func(event record.Event) error {
		if event.Type == "recovery" {
			recoveries++
		}
		return nil
	})
	if err != nil || summary.Reason != "canceled" || waits != 9 || recoveries != 2 || commandCount(a.commands, "ATSP0") != 3 || commandCount(a.commands, "03") != 0 || len(a.commands) != commandsAtCancel || !a.closed {
		t.Fatalf("standby escalation/cancellation: err=%v summary=%+v waits=%d recoveries=%d commands=%v", err, summary, waits, recoveries, a.commands)
	}
}
