package probe

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestStreamWaitsForECUWithoutRepeatedDiscovery(t *testing.T) {
	t.Parallel()
	for _, response := range []string{"NO DATA\r>", "UNABLE TO CONNECT\r>"} {
		t.Run(strings.TrimSpace(response), func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			bitmap := a.responses["0100"]
			a.responses["0100"] = response
			r, clock := recordingRunner(t, a, 0)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			waits, scans := 0, 0
			r.wait = func(ctx context.Context, d time.Duration) error {
				if d != 10*time.Second {
					t.Errorf("standby delay = %s", d)
				}
				*clock = clock.Add(d)
				waits++
				if waits == 3 {
					a.responses["0100"] = bitmap
				}
				return ctx.Err()
			}
			standby := false
			summary, err := r.Stream(ctx, func(e record.Event) error {
				if e.Type == "standby" {
					standby = true
				}
				if e.Type == "capabilities" {
					scans++
					cancel()
				}
				return nil
			})
			if err != nil || !standby || waits != 3 || scans != 1 || summary.Reason != "canceled" {
				t.Fatalf("standby/recovery: %+v err=%v standby=%t waits=%d scans=%d", summary, err, standby, waits, scans)
			}
			if commandCount(a.commands, "03") != 1 || commandCount(a.commands, "ATSP0") != 2 {
				t.Fatalf("repeated full scan while ECU absent: %v", a.commands)
			}
		})
	}
}

func TestStreamDetectsIgnitionOffAndResumesNewSegment(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, clock := recordingRunner(t, a, 0)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	ready, standby, resumed := 0, 0, false
	good := a.responses["0100"]
	rounds := 0
	r.wait = func(ctx context.Context, d time.Duration) error {
		*clock = clock.Add(d)
		if standby > 0 {
			a.responses["0100"] = good
		}
		return ctx.Err()
	}
	summary, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "capabilities" {
			ready++
			if ready == 1 {
				for command := range a.responses {
					if strings.HasPrefix(command, "01") {
						a.responses[command] = "NO DATA\r>"
					}
				}
			}
			if ready == 2 {
				resumed = true
				cancel()
			}
		}
		if ready == 1 && e.Type == "command" && e.Data.(RecordingCommand).Command == "010C" {
			rounds++
		}
		if e.Type == "standby" {
			standby++
			if rounds != 3 {
				t.Errorf("standby after %d failed rounds", rounds)
			}
		}
		return nil
	})
	if err != nil || !resumed || standby != 1 || summary.Segments != 2 || summary.Gaps != 1 {
		t.Fatalf("ignition cycle: %+v %v resumed=%t standby=%d", summary, err, resumed, standby)
	}
}

func TestStreamZeroRPMAndOneMissingPIDDoNotEnterStandby(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["010C"] = "410C0000\r>"
	a.responses["010B"] = "NO DATA\r>"
	r, clock := recordingRunner(t, a, 0)
	started := *clock
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.wait = func(ctx context.Context, d time.Duration) error {
		*clock = clock.Add(d)
		if clock.Sub(started) > 5*time.Second {
			cancel()
		}
		return ctx.Err()
	}
	var standby bool
	_, err := r.Stream(ctx, func(e record.Event) error { standby = standby || e.Type == "standby"; return nil })
	if err != nil || standby {
		t.Fatalf("zero RPM or one absent PID treated as ignition off: %v standby=%t", err, standby)
	}
}

func commandCount(commands []string, command string) int {
	count := 0
	for _, code := range commands {
		if code == command {
			count++
		}
	}
	return count
}

func TestStreamRetriesZeroContinuesInSparseBursts(t *testing.T) {
	t.Parallel()
	r, clock := recordingRunner(t, fixtureAdapter(t), 0)
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) { opens++; return nil, io.EOF }
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	var delays []time.Duration
	r.wait = func(ctx context.Context, d time.Duration) error {
		delays = append(delays, d)
		*clock = clock.Add(d)
		if len(delays) == 3 {
			cancel()
		}
		return ctx.Err()
	}
	summary, err := r.Stream(ctx, func(record.Event) error { return nil })
	if err != nil || summary.Reason != "canceled" || opens != 3 || !slices.Equal(delays, []time.Duration{10 * time.Second, 10 * time.Second, 10 * time.Second}) {
		t.Fatalf("retry bursts: %+v %v opens=%d delays=%v", summary, err, opens, delays)
	}
}

func TestStreamReopenStillRequiresPromptAfterIdlePause(t *testing.T) {
	t.Parallel()
	first, second := fixtureAdapter(t), fixtureAdapter(t)
	first.failCommand, first.disconnect = "010C", true
	r, clock := recordingRunner(t, first, 0)
	r.config.RecoveryTimeout = time.Millisecond
	opens, pauses := 0, 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return first, nil
		}
		return second, nil
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	r.wait = func(ctx context.Context, d time.Duration) error {
		*clock = clock.Add(d)
		pauses++
		if pauses == 2 {
			if len(second.commands) != 0 {
				t.Fatal("wrote before consuming old prompt")
			}
			second.pending = []byte(">")
		}
		return ctx.Err()
	}
	_, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "capabilities" {
			cancel()
		}
		return nil
	})
	if err != nil || opens != 2 || pauses != 2 || !first.closed || !second.closed || len(second.commands) == 0 {
		t.Fatalf("prompt recovery: %v opens=%d pauses=%d commands=%v", err, opens, pauses, second.commands)
	}
}

func TestStreamFatalOpenErrorsAreNotRetried(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{os.ErrPermission, errors.New("invalid helper protocol")} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			r, _ := recordingRunner(t, fixtureAdapter(t), 2)
			opens := 0
			r.open = func(context.Context) (elm327.Transport, error) { opens++; return nil, cause }
			summary, err := r.Stream(t.Context(), func(record.Event) error { return nil })
			if !errors.Is(err, cause) || opens != 1 || summary.Reason != "failed" {
				t.Fatalf("fatal error retried/hidden: %+v %v opens=%d", summary, err, opens)
			}
		})
	}
}

func TestStreamFatalErrorsJoinedToTemporaryFailureAreNotHidden(t *testing.T) {
	t.Parallel()
	for _, transport := range []string{"ble", "serial"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()
			r, _ := recordingRunner(t, fixtureAdapter(t), 0)
			r.config.Transport = transport
			cause := errors.Join(os.ErrNotExist, os.ErrPermission)
			opens := 0
			r.open = func(context.Context) (elm327.Transport, error) { opens++; return nil, cause }
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			_, err := r.Stream(ctx, func(record.Event) error { return nil })
			if !errors.Is(err, os.ErrPermission) || opens != 1 {
				t.Fatalf("fatal joined error hidden: %v opens=%d", err, opens)
			}
		})
	}
}

func TestStreamBurstBackoffIsBounded(t *testing.T) {
	t.Parallel()
	r, clock := recordingRunner(t, fixtureAdapter(t), 5)
	r.open = func(context.Context) (elm327.Transport, error) { return nil, blewire.ErrUnavailable }
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var delays []time.Duration
	r.wait = func(ctx context.Context, d time.Duration) error {
		*clock = clock.Add(d)
		delays = append(delays, d)
		if len(delays) == 7 {
			cancel()
		}
		return ctx.Err()
	}
	_, err := r.Stream(ctx, func(record.Event) error { return nil })
	want := []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second, 4 * time.Second, 10 * time.Second, 500 * time.Millisecond}
	if err != nil || !slices.Equal(delays, want) {
		t.Fatalf("unbounded burst: %v %v", err, delays)
	}
}

func TestStreamNoCoreChecksECUInsteadOfAdapterOnly(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0100"] = "410000000000\r>"
	r, clock := recordingRunner(t, a, 0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	waits, probes := 0, 0
	r.wait = func(ctx context.Context, d time.Duration) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if d != 10*time.Second {
			t.Errorf("no-core wait: %v", d)
		}
		*clock = clock.Add(d)
		waits++
		return ctx.Err()
	}
	_, err := r.Stream(ctx, func(e record.Event) error {
		if waits > 0 && e.Type == "command" && e.Data.(RecordingCommand).Command == "0100" {
			probes++
			a.responses["0100"] = "NO DATA\r>"
		}
		if e.Type == "standby" {
			cancel()
		}
		return nil
	})
	if err != nil || probes != 2 || waits != 2 || commandCount(a.commands, "010C") != 0 {
		t.Fatalf("no-core ECU monitoring: err=%v probes=%d waits=%d commands=%v", err, probes, waits, a.commands)
	}
}

func TestStreamTimeoutKeepsSessionAndRecoversBeforeWriting(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand, a.latePrompt = "010C", true
	r, _ := recordingRunner(t, a, 0)
	r.config.Timeout = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opens, recoveries := 0, 0
	r.open = func(context.Context) (elm327.Transport, error) { opens++; return a, nil }
	_, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "recovery" {
			recoveries++
		}
		if e.Type == "capabilities" {
			cancel()
		}
		return nil
	})
	if err != nil || opens != 1 || recoveries != 1 || commandCount(a.commands, "ATE0") != 2 || !a.closed {
		t.Fatalf("unsafe timeout recovery: %v opens=%d recoveries=%d commands=%v", err, opens, recoveries, a.commands)
	}
}

func TestStreamStandbySinkFailureStopsBeforeNextQuery(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0100"] = "NO DATA\r>"
	r, _ := recordingRunner(t, a, 0)
	cause := errors.New("sink failed")
	summary, err := r.Stream(t.Context(), func(e record.Event) error {
		if e.Type == "standby" {
			return cause
		}
		return nil
	})
	if !errors.Is(err, cause) || summary.Reason != "output_error" || commandCount(a.commands, "0100") != 1 || !a.closed {
		t.Fatalf("sink failure ignored: %+v %v %v", summary, err, a.commands)
	}
}

func TestStreamIdleWaitCancelsWithoutAnotherQuery(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0100"] = "NO DATA\r>"
	r, _ := recordingRunner(t, a, 0)
	r.wait = wait
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := make(chan struct{})
	type result struct {
		summary RecordingSummary
		err     error
	}
	done := make(chan result, 1)
	go func() {
		summary, err := r.Stream(ctx, func(e record.Event) error {
			if e.Type == "standby" {
				close(ready)
			}
			return nil
		})
		done <- result{summary, err}
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("did not enter standby")
	}
	cancel()
	select {
	case got := <-done:
		if got.err != nil || got.summary.Reason != "canceled" || commandCount(a.commands, "0100") != 1 || !a.closed {
			t.Fatalf("idle cancellation: %+v %v commands=%v", got.summary, got.err, a.commands)
		}
	case <-time.After(time.Second):
		t.Fatal("idle wait ignored cancellation")
	}
}

func TestStreamReconnectCleanupFailureIsTerminal(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand, a.disconnect = "010C", true
	r, _ := recordingRunner(t, a, 2)
	cause := errors.New("cleanup failed")
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		return closeErrorTransport{adapter: a, cause: cause}, nil
	}
	summary, err := r.Stream(t.Context(), func(record.Event) error { return nil })
	if !errors.Is(err, cause) || opens != 1 || summary.Reason != "failed" || !a.closed {
		t.Fatalf("cleanup hidden: %+v %v opens=%d", summary, err, opens)
	}
}
