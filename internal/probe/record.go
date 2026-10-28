package probe

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

var errRecordingDuration = errors.New("recording duration reached")

type recording struct {
	r           *Runner
	started     time.Time
	duration    time.Duration
	sink        func(record.Event) error
	summary     RecordingSummary
	report      Report
	session     *elm327.Session
	outputErr   error
	needSync    bool
	phase       string
	segmentOpen bool
}

// Record keeps only current capabilities/readings. Historical commands are sent
// synchronously to sink; a sink failure prevents the next ECU command.
func (r *Runner) Record(parent context.Context, duration time.Duration, sink func(record.Event) error) (RecordingSummary, error) {
	if duration <= 0 || duration > 24*time.Hour || sink == nil {
		return RecordingSummary{}, fmt.Errorf("recording requires duration in (0,24h] and an event sink")
	}
	ctx, cancel := context.WithTimeout(parent, duration)
	defer cancel()
	return r.runStream(ctx, parent, duration, sink)
}

// Stream shares the recorder's polling/recovery without a recording time limit.
func (r *Runner) Stream(ctx context.Context, sink func(record.Event) error) (RecordingSummary, error) {
	if sink == nil {
		return RecordingSummary{}, fmt.Errorf("live stream requires an event sink")
	}
	return r.runStream(ctx, ctx, 0, sink)
}

func (r *Runner) runStream(ctx, parent context.Context, duration time.Duration, sink func(record.Event) error) (RecordingSummary, error) {
	runner := *r
	runner.config.ScanSupported = true
	runner.origin = runner.now()
	s := &recording{r: &runner, started: runner.origin, duration: duration, sink: sink}
	runner.observer = s.observe
	err := s.emit("recording_start", recordingMetadata(runner.config, duration))
	if err == nil {
		err = s.run(ctx)
	}
	return s.finish(ctx, parent, err)
}

func (s *recording) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.duration > 0 && s.r.now().Sub(s.started) >= s.duration {
		return errRecordingDuration
	}
	return nil
}

func (s *recording) run(ctx context.Context) error {
	for attempt := 0; attempt <= s.r.config.Retries; attempt++ {
		if err := s.check(ctx); err != nil {
			return err
		}
		s.summary.Segments++
		s.report = Report{StartedAt: s.started.UTC(), streaming: true}
		resetReport(&s.report, s.r.config, attempt+1)
		if err := s.emit("segment_start", map[string]any{"transport": s.report.Transport, "source": s.report.Source, "device": s.report.Device}); err != nil {
			return err
		}
		s.segmentOpen = true
		err := s.attempt(ctx)
		if err == nil {
			return nil
		}
		if s.outputErr != nil || ctx.Err() != nil || errors.Is(err, errRecordingDuration) {
			return err
		}
		s.summary.Gaps++
		if outputErr := s.emit("gap", map[string]any{"error": err.Error(), "phase": s.phase}); outputErr != nil {
			return outputErr
		}
		link, isLink := errors.AsType[*linkError](err)
		canRetry := isLink || s.phase == "open" || s.phase == "synchronize"
		if attempt == s.r.config.Retries || !canRetry {
			return err
		}
		if isLink && link.status == elm327.Timeout {
			if recoveryErr := s.r.recover(ctx, s.session, &s.report); recoveryErr == nil {
				if err := s.endSegment("recovered"); err != nil {
					return err
				}
				if err := s.backoff(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			if s.outputErr != nil {
				return s.outputErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
		if s.session != nil {
			if closeErr := s.close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
			s.needSync = true
		}
		if err := s.endSegment("reconnect"); err != nil {
			return err
		}
		if err := s.backoff(ctx, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("recording retry budget exhausted")
}

func (s *recording) attempt(ctx context.Context) error {
	if s.session == nil {
		s.phase = "open"
		wire, err := s.r.openPort(ctx, &s.report)
		if err != nil {
			return err
		}
		s.session = elm327.NewSession(wire)
		if s.needSync {
			s.phase = "synchronize"
			if err := s.r.recover(ctx, s.session, &s.report); err != nil {
				return err
			}
			s.needSync = false
		}
	}
	s.phase = "discovery"
	if err := s.r.scan(ctx, s.session, &s.report); err != nil {
		return err
	}
	capabilities := s.report
	capabilities.Capabilities = slices.Clone(s.report.Capabilities)
	capabilities.Bitmaps = slices.Clone(s.report.Bitmaps)
	if err := s.emit("capabilities", RecordingCapabilities{Report: capabilities, MissingInputs: missingCore(s.report)}); err != nil {
		return err
	}
	s.phase = "polling"
	started := s.r.now()
	err := s.poll(ctx)
	s.summary.PollingNS += s.r.now().Sub(started).Nanoseconds()
	return err
}

func (s *recording) poll(ctx context.Context) error {
	schedule := newPollSchedule(s.report, s.r.now(), s.duration == 0)
	for {
		if err := s.check(ctx); err != nil {
			return err
		}
		roundStart := s.r.now()
		for _, pid := range schedule.core {
			if err := s.check(ctx); err != nil {
				return err
			}
			if err := s.r.readPID(ctx, s.session, &s.report, pid); err != nil {
				return err
			}
		}
		if command := schedule.support(s.r.now()); command != "" {
			if err := s.check(ctx); err != nil {
				return err
			}
			err := s.readScheduled(ctx, command)
			if err != nil {
				return err
			}
			schedule.complete(command, s.r.now())
		}
		delay := max(0, roundStart.Add(time.Second).Sub(s.r.now()))
		if s.duration > 0 {
			delay = min(delay, max(0, s.duration-s.r.now().Sub(s.started)))
		}
		if err := s.r.wait(ctx, delay); err != nil {
			return err
		}
	}
}

func (s *recording) backoff(ctx context.Context, attempt int) error {
	delay := min(4*time.Second, 500*time.Millisecond*time.Duration(1<<min(attempt, 3)))
	if err := s.emit("recovery", map[string]any{"phase": "backoff", "delay_ns": delay.Nanoseconds()}); err != nil {
		return err
	}
	return s.r.wait(ctx, delay)
}

func (s *recording) endSegment(reason string) error {
	if !s.segmentOpen {
		return nil
	}
	s.segmentOpen = false
	return s.emit("segment_end", map[string]string{"reason": reason})
}

func (s *recording) close() error {
	if s.session == nil {
		return nil
	}
	err := s.session.Close()
	s.session = nil
	return err
}

func (s *recording) finish(ctx, parent context.Context, err error) (RecordingSummary, error) {
	s.summary.Reason = "failed"
	switch {
	case s.outputErr != nil:
		s.summary.Reason = "output_error"
		if errors.Is(s.outputErr, record.ErrSizeLimit) {
			s.summary.Reason = "size_limit"
		}
	case errors.Is(err, errRecordingDuration):
		s.summary.Reason, err = "duration", nil
	case ctx.Err() != nil && errors.Is(err, ctx.Err()):
		s.summary.Reason, err = "duration", nil
		if parent.Err() != nil {
			s.summary.Reason = "canceled"
		}
	}
	closeErr := s.close()
	if closeErr != nil {
		err = errors.Join(err, closeErr)
		s.summary.Reason = "failed"
	}
	if s.summary.Segments > 0 {
		err = errors.Join(err, s.endSegment(s.summary.Reason))
	}
	if err != nil {
		s.summary.Error = err.Error()
	}
	s.summary.DurationNS = s.r.now().Sub(s.started).Nanoseconds()
	s.summary.UnobservedNS = max(0, s.summary.DurationNS-s.summary.PollingNS)
	now := s.r.now()
	finalErr := s.sink(record.Event{Type: "recording_end", Timestamp: now.UTC(), ElapsedNS: now.Sub(s.started).Nanoseconds(), Segment: s.summary.Segments, Data: s.summary})
	if finalErr != nil && s.summary.Reason != "size_limit" {
		s.summary.Reason = "output_error"
	}
	return s.summary, errors.Join(err, finalErr)
}

func (s *recording) readScheduled(ctx context.Context, command string) error {
	if command == "ATRV" {
		return s.r.voltage(ctx, s.session, &s.report)
	}
	return s.r.readPID(ctx, s.session, &s.report, itemPID(command))
}
