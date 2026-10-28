package probe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

const standbyInterval = 10 * time.Second

var errECUWaiting = errors.New("ecu is not responding")

// Standby is a continuous-LIVE lifecycle event, never emitted by finite Record.
// Reason is ecu, adapter or synchronize; it is not an ignition measurement.
type Standby struct {
	Reason string `json:"reason"`
}

func (s *recording) runLive(ctx context.Context) error {
	for attempt := 0; ; {
		if err := s.check(ctx); err != nil {
			return err
		}
		if err := s.startLiveSegment(attempt); err != nil {
			return err
		}
		err := s.attempt(ctx)
		if err == nil || s.outputErr != nil || ctx.Err() != nil {
			return err
		}
		s.summary.Gaps++
		if outputErr := s.emit("gap", map[string]string{"error": err.Error(), "phase": s.phase}); outputErr != nil {
			return outputErr
		}
		if errors.Is(err, errECUWaiting) {
			if err := s.endSegment("ecu_wait"); err != nil {
				return err
			}
			err = s.waitECU(ctx)
			if err == nil {
				attempt = 0
				continue
			}
		}
		if s.outputErr != nil || ctx.Err() != nil || !s.retryableLive(err) {
			return err
		}
		reason := "adapter"
		if s.session != nil {
			s.needSync = true
			if errors.Is(err, context.DeadlineExceeded) {
				reason = "synchronize"
			} else if closeErr := s.close(); closeErr != nil {
				return errors.Join(err, closeErr)
			}
		}
		if err := s.endSegment(reason); err != nil {
			return err
		}
		if err := s.emit("standby", Standby{Reason: reason}); err != nil {
			return err
		}
		delay := standbyInterval
		if attempt < s.r.config.Retries {
			delay = min(4*time.Second, 500*time.Millisecond*time.Duration(1<<min(attempt, 3)))
			attempt++
		} else {
			attempt = 0
		}
		if err := s.r.wait(ctx, delay); err != nil {
			return err
		}
	}
}

func (s *recording) startLiveSegment(attempt int) error {
	s.summary.Segments++
	s.report = Report{StartedAt: s.started.UTC(), streaming: true}
	resetReport(&s.report, s.r.config, attempt+1)
	if err := s.emit("segment_start", map[string]any{"transport": s.report.Transport, "source": s.report.Source, "device": s.report.Device}); err != nil {
		return err
	}
	s.segmentOpen = true
	return nil
}

func (s *recording) scanLive(ctx context.Context, session *elm327.Session, report *Report) error {
	if err := s.r.initialize(ctx, session, report); err != nil {
		return err
	}
	if err := s.r.discover(ctx, session, report); err != nil {
		return err
	}
	if len(report.Bitmaps) == 0 || report.Bitmaps[0].Status != "available" {
		return errECUWaiting
	}
	return s.r.scanReadings(ctx, session, report)
}

func (s *recording) waitECU(ctx context.Context) error {
	s.phase = "standby"
	if err := s.emit("standby", Standby{Reason: "ecu"}); err != nil {
		return err
	}
	for {
		if err := s.r.wait(ctx, standbyInterval); err != nil {
			return err
		}
		response, err := s.r.command(ctx, s.session, &s.report, "ATI")
		if err != nil {
			return err
		}
		identity := strings.ToUpper(strings.Join(response.Lines, " "))
		if response.Status != elm327.OK || !strings.Contains(identity, "ELM") && !strings.Contains(identity, "STN") {
			return fmt.Errorf("adapter standby check: unexpected identity or status %s", response.Status)
		}
		ready, err := s.ecuReady(ctx)
		if err != nil || ready {
			return err
		}
	}
}

func (s *recording) ecuReady(ctx context.Context) (bool, error) {
	response, err := s.r.command(ctx, s.session, &s.report, "0100")
	if err != nil {
		return false, err
	}
	_, decodeErr := elm327.DecodeSupported(0, response)
	return decodeErr == nil, nil
}

func (s *recording) pollWithoutCore(ctx context.Context, schedule *pollSchedule) error {
	for {
		if err := s.r.wait(ctx, standbyInterval); err != nil {
			return err
		}
		ready, err := s.ecuReady(ctx)
		if err != nil {
			return err
		}
		if !ready {
			return errECUWaiting
		}
		if command := schedule.support(s.r.now()); command != "" {
			if err := s.readScheduled(ctx, command); err != nil {
				return err
			}
			schedule.complete(command, s.r.now())
		}
	}
}

func (s *recording) retryableLive(err error) bool {
	return transientTransport(err, s.phase == "open" && s.r.config.Transport == "serial")
}

// Every member of a joined error must be transient: a timeout joined with a
// cleanup/permission failure must not hide that terminal failure.
func transientTransport(err error, missingSerial bool) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !transientTransport(cause, missingSerial) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return transientTransport(wrapped.Unwrap(), missingSerial)
	}
	return missingSerial && errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) ||
		errors.Is(err, blewire.ErrDisconnected) || errors.Is(err, blewire.ErrUnavailable) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO) ||
		errors.Is(err, syscall.ENXIO) || errors.Is(err, syscall.ENODEV) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE)
}
