package probe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func recordingRunner(t *testing.T, a *adapter, retries int) (*Runner, *time.Time) {
	t.Helper()
	r, err := New(Config{Transport: "ble", Timeout: time.Second, RecoveryTimeout: 100 * time.Millisecond, Retries: retries}, func(context.Context) (elm327.Transport, error) { return a, nil })
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	r.now = func() time.Time { return now }
	r.wait = func(ctx context.Context, d time.Duration) error { now = now.Add(d); return ctx.Err() }
	return r, &now
}

func TestRecordStreamsDiscoveryAndSamplesWithoutHistory(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0120"] = "4120C0000000\r>"
	a.responses["010B"] = "410B29\r>"
	a.responses["010F"] = "410F39\r>"
	a.responses["0113"] = "411303\r>"
	r, clock := recordingRunner(t, a, 0)
	var events []record.Event
	summary, err := r.Record(t.Context(), 20*time.Second, func(e record.Event) error {
		events = append(events, e)
		if e.Type == "command" {
			*clock = clock.Add(20 * time.Millisecond)
		}
		if e.Type == "capabilities" {
			data := e.Data.(RecordingCapabilities)
			if len(data.Report.Commands) != 0 || len(data.Report.Warnings) != 0 {
				t.Fatal("accumulated audit/warnings")
			}
		}
		return nil
	})
	if err != nil || summary.Reason != "duration" || summary.Samples < 30 || summary.DurationNS < int64(20*time.Second) || !a.closed {
		t.Fatalf("summary=%+v err=%v closed=%t", summary, err, a.closed)
	}
	if events[0].Type != "recording_start" || events[len(events)-1].Type != "recording_end" {
		t.Fatal("missing lifecycle")
	}
	for _, blocked := range []string{"ATZ", "0110", "012F", "0140", "0142", "015E", "04", "0902"} {
		if slices.Contains(a.commands, blocked) {
			t.Fatalf("unsafe command: %s", blocked)
		}
	}
	var rpm, rawOnly, noData bool
	for _, e := range events {
		if e.Type != "command" {
			continue
		}
		data := e.Data.(RecordingCommand)
		if data.CompletedElapsedNS < data.StartedElapsedNS || data.LatencyNS < 0 {
			t.Fatal("negative command timing")
		}
		if data.Command == "010C" && data.Result != nil && data.Result.Value != nil {
			rpm = true
		}
		if data.Command == "0121" && data.Result != nil && data.Result.Status == "temporarily_unavailable" {
			noData = true
		}
		if data.Command == "0113" && data.Result != nil && data.Result.Status == "raw_only" {
			rawOnly = true
		}
	}
	if !rpm || !rawOnly || !noData {
		t.Fatalf("incomplete sample evidence: rpm=%t raw=%t noData=%t", rpm, rawOnly, noData)
	}
	if r.observer != nil || r.config.ScanSupported {
		t.Fatal("Record mutated original Runner")
	}
}

func TestRecordSinkFailureStopsBeforeNextCommand(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 2)
	cause := errors.New("disk full")
	summary, err := r.Record(t.Context(), time.Minute, func(e record.Event) error {
		if e.Type == "command" {
			return cause
		}
		return nil
	})
	if !errors.Is(err, cause) || summary.Reason != "output_error" || len(a.commands) != 1 || !a.closed {
		t.Fatalf("continued after sink error: %+v %v %v closed=%t", summary, err, a.commands, a.closed)
	}
}

func TestRecordCancellationIsCleanAndKeepsFinalEvent(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var types []string
	summary, err := r.Record(ctx, time.Minute, func(e record.Event) error {
		types = append(types, e.Type)
		if e.Type == "capabilities" {
			cancel()
		}
		return nil
	})
	if err != nil || summary.Reason != "canceled" || types[len(types)-1] != "recording_end" || !a.closed {
		t.Fatalf("cancel: %+v %v %v", summary, err, types)
	}
}

func TestRecordTimeoutRequiresRecoveryBeforeReinitialization(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand, a.latePrompt = "010C", true
	r, _ := recordingRunner(t, a, 1)
	var types []string
	summary, err := r.Record(t.Context(), 3*time.Second, func(e record.Event) error { types = append(types, e.Type); return nil })
	if err != nil || summary.Gaps != 1 || summary.Segments != 2 || !slices.Contains(types, "recovery") {
		t.Fatalf("recovery: %+v %v %v", summary, err, types)
	}
	if count := strings.Count(strings.Join(a.commands, ","), "ATE0"); count != 2 {
		t.Fatalf("not reinitialized: %v", a.commands)
	}
}

func TestRecordReopenRequiresPromptAndRetryBudget(t *testing.T) {
	t.Parallel()
	first := fixtureAdapter(t)
	first.failCommand, first.disconnect = "010C", true
	second := fixtureAdapter(t)
	second.pending = []byte(">")
	r, _ := recordingRunner(t, first, 1)
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return first, nil
		}
		return second, nil
	}
	summary, err := r.Record(t.Context(), 2*time.Second, func(record.Event) error { return nil })
	if err != nil || opens != 2 || summary.Gaps != 1 || !first.closed || !second.closed {
		t.Fatalf("reopen: %+v %v opens=%d", summary, err, opens)
	}
	r.open = func(context.Context) (elm327.Transport, error) { return nil, io.EOF }
	summary, err = r.Record(t.Context(), time.Minute, func(record.Event) error { return nil })
	if !errors.Is(err, io.EOF) || summary.Segments != 2 || summary.Reason != "failed" {
		t.Fatalf("budget: %+v %v", summary, err)
	}
}

func TestRepeatedPIDNoDataClearsPriorValue(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	var report Report
	resetReport(&report, r.config, 1)
	session := elm327.NewSession(a)
	if err := r.readPID(t.Context(), session, &report, 5); err != nil {
		t.Fatal(err)
	}
	a.responses["0105"] = "NO DATA\r>"
	if err := r.readPID(t.Context(), session, &report, 5); err != nil {
		t.Fatal(err)
	}
	if c := capability(t, report, 5); c.Value != nil || c.Status != "temporarily_unavailable" {
		t.Fatalf("stale value survived: %+v", c)
	}
}

func TestRecordPreservesHistoryAndClosesEachSegmentOnce(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand, a.latePrompt = "010C", true
	r, _ := recordingRunner(t, a, 1)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var voltage *RecordingReading
	ends := 0
	summary, err := r.Record(ctx, time.Minute, func(e record.Event) error {
		if e.Type == "command" {
			data := e.Data.(RecordingCommand)
			if data.Command == "ATRV" {
				voltage = data.Result
			}
		}
		if e.Type == "segment_end" {
			ends++
			cancel()
		}
		return nil
	})
	if err != nil || summary.Reason != "canceled" || ends != 1 || voltage == nil || voltage.Value == nil || *voltage.Value != 14.2 {
		t.Fatalf("history/boundaries: %+v %v ends=%d voltage=%+v", summary, err, ends, voltage)
	}
}

func TestRecordNeverResetsTotalRetryBudget(t *testing.T) {
	t.Parallel()
	r, clock := recordingRunner(t, fixtureAdapter(t), 1)
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		a := fixtureAdapter(t)
		a.failCommand, a.disconnect = "010C", true
		if opens > 1 {
			a.pending = []byte(">")
		}
		return a, nil
	}
	var hadAvailable bool
	summary, err := r.Record(t.Context(), time.Minute, func(e record.Event) error {
		if e.Type == "command" {
			*clock = clock.Add(time.Millisecond)
			if e.Data.(RecordingCommand).Command == "ATRV" {
				hadAvailable = true
			}
		}
		return nil
	})
	if err == nil || !hadAvailable || opens != 2 || summary.Segments != 2 || summary.Gaps != 2 {
		t.Fatalf("unbounded retries: %+v err=%v opens=%d", summary, err, opens)
	}
}

type cancelOnWrite struct {
	*adapter
	cancel context.CancelFunc
}

func (a cancelOnWrite) Write(ctx context.Context, p []byte) (int, error) {
	n, err := a.adapter.Write(ctx, p)
	if string(p) == "010C\r" {
		a.cancel()
	}
	return n, err
}

func TestRecordCancellationDuringCommandPreservesPartialAudit(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand = "010C"
	r, _ := recordingRunner(t, a, 2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r.open = func(context.Context) (elm327.Transport, error) { return cancelOnWrite{adapter: a, cancel: cancel}, nil }
	var interrupted bool
	summary, err := r.Record(ctx, time.Minute, func(e record.Event) error {
		if e.Type == "command" {
			c := e.Data.(RecordingCommand)
			if c.Command == "010C" && c.Error != "" {
				interrupted = true
			}
		}
		return nil
	})
	if err != nil || !interrupted || summary.Reason != "canceled" || summary.Segments != 1 || !a.closed {
		t.Fatalf("cancel in command: %+v %v interrupted=%t", summary, err, interrupted)
	}
}

func TestRecordReopenWithoutPromptSendsNoCommands(t *testing.T) {
	t.Parallel()
	first, second := fixtureAdapter(t), fixtureAdapter(t)
	first.failCommand, first.disconnect = "010C", true
	r, _ := recordingRunner(t, first, 1)
	opens := 0
	r.open = func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return first, nil
		}
		return second, nil
	}
	summary, err := r.Record(t.Context(), time.Minute, func(record.Event) error { return nil })
	if err == nil || opens != 2 || len(second.commands) != 0 || !second.closed || summary.Reason != "failed" {
		t.Fatalf("unsafe reopen: %+v %v commands=%v", summary, err, second.commands)
	}
}

type closeErrorTransport struct {
	*adapter
	cause error
}

func (a closeErrorTransport) Close() error { return errors.Join(a.adapter.Close(), a.cause) }

func TestRecordCloseFailureIsNotCleanCancellation(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cause := errors.New("close failed")
	r.open = func(context.Context) (elm327.Transport, error) {
		return closeErrorTransport{adapter: a, cause: cause}, nil
	}
	summary, err := r.Record(ctx, time.Minute, func(e record.Event) error {
		if e.Type == "capabilities" {
			cancel()
		}
		return nil
	})
	if !errors.Is(err, cause) || summary.Reason != "failed" || !a.closed {
		t.Fatalf("close error hidden: %+v %v", summary, err)
	}
}

func TestRecordingCommandAlwaysContainsTimingFields(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(RecordingCommand{})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"started_elapsed_ns", "completed_elapsed_ns", "latency_ns"} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("zero timing field missing: %s", data)
		}
	}
}

func TestLongRecordingDoesNotAccumulateAuditOrWarnings(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	r.config.ScanSupported = true
	r.origin = r.now()
	s := &recording{r: r, started: r.origin, duration: time.Hour, sink: func(record.Event) error { return nil }}
	r.observer = s.observe
	t.Cleanup(func() {
		if err := s.close(); err != nil {
			t.Error(err)
		}
	})
	if err := s.run(t.Context()); !errors.Is(err, errRecordingDuration) {
		t.Fatal(err)
	}
	if s.summary.Commands < 10000 || len(s.report.Commands) != 0 || len(s.report.Warnings) != 0 || len(s.report.Capabilities) != 255 || len(s.report.Bitmaps) > 8 {
		t.Fatalf("unbounded trip history: commands=%d retained=%d warnings=%d", s.summary.Commands, len(s.report.Commands), len(s.report.Warnings))
	}
}

func TestLiveStreamHasNoRecordingDurationLimit(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, clock := recordingRunner(t, a, 0)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready, readAfter := false, false
	summary, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "capabilities" {
			ready = true
			*clock = clock.Add(25 * time.Hour)
		}
		if ready && e.Type == "command" && e.Data.(RecordingCommand).Command == "010C" {
			readAfter = true
			cancel()
		}
		return nil
	})
	if err != nil || !readAfter || summary.Reason != "canceled" || summary.DurationNS < int64(25*time.Hour) || !a.closed {
		t.Fatalf("stream stopped on recorder duration: %+v %v read=%t", summary, err, readAfter)
	}
}

func TestRecordingSummaryRetainsCleanupError(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	cause := errors.New("cleanup evidence")
	r.open = func(context.Context) (elm327.Transport, error) {
		return closeErrorTransport{adapter: a, cause: cause}, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var final RecordingSummary
	summary, err := r.Stream(ctx, func(e record.Event) error {
		if e.Type == "capabilities" {
			cancel()
		}
		if e.Type == "recording_end" {
			final = e.Data.(RecordingSummary)
		}
		return nil
	})
	if !errors.Is(err, cause) || !strings.Contains(summary.Error, "cleanup evidence") || final.Error != summary.Error || final.Reason != "failed" {
		t.Fatalf("cleanup evidence missing: %+v %+v %v", summary, final, err)
	}
}
