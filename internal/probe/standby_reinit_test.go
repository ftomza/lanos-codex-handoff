package probe

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
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

type writeFailureTransport struct {
	elm327.Transport
	cause error
}

func (w writeFailureTransport) Write(context.Context, []byte) (int, error) { return 0, w.cause }

func TestStreamNativeTimeoutRetriesAfterClosedTransport(t *testing.T) {
	t.Parallel()
	first, second := fixtureAdapter(t), fixtureAdapter(t)
	second.pending = []byte(">")
	r, clock := recordingRunner(t, first, 0)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	opens, waits, ready := 0, 0, false
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return writeFailureTransport{Transport: first, cause: blewire.ErrTimeout}, nil
		}
		if !first.closed {
			t.Fatal("reopened transport before closing old helper")
		}
		return second, nil
	}
	r.wait = func(ctx context.Context, delay time.Duration) error {
		if delay != 10*time.Second {
			t.Errorf("retry ignored sparse bound: %s", delay)
		}
		*clock = clock.Add(delay)
		waits++
		return ctx.Err()
	}
	_, err := r.Stream(ctx, func(event record.Event) error {
		if event.Type == "capabilities" {
			ready = true
			cancel()
		}
		return nil
	})
	if err != nil || !ready || opens != 2 || waits != 1 || !second.closed {
		t.Fatalf("native timeout was terminal: %v ready=%t opens=%d waits=%d", err, ready, opens, waits)
	}
}

func TestStreamNativeTimeoutJoinedFatalRemainsTerminal(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		return nil, errors.Join(blewire.ErrTimeout, os.ErrPermission)
	}
	_, err := r.Stream(t.Context(), func(record.Event) error { return nil })
	if opens != 1 || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("fatal cause hidden: %v opens=%d", err, opens)
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
