package probe

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

type Config struct {
	Transport       string        `json:"transport"`
	BLEName         string        `json:"ble_name"`
	BLEHelper       string        `json:"ble_helper,omitzero"`
	ConnectTimeout  time.Duration `json:"connect_timeout_ns"`
	AdapterModel    string        `json:"adapter_model"`
	Port            string        `json:"port,omitzero"`
	Baud            int           `json:"baud,omitzero"`
	Timeout         time.Duration `json:"timeout_ns"`
	InitialTimeout  time.Duration `json:"initial_timeout_ns"`
	RecoveryTimeout time.Duration `json:"recovery_timeout_ns"`
	Retries         int           `json:"retries"`
	ScanSupported   bool          `json:"scan_supported"`
}

type Runner struct {
	config   Config
	open     func(context.Context) (elm327.Transport, error)
	wait     func(context.Context, time.Duration) error
	now      func() time.Time
	observer func(CommandLog) error
	origin   time.Time
}

func New(cfg Config, open func(context.Context) (elm327.Transport, error)) (*Runner, error) {
	if cfg.Transport == "" {
		cfg.Transport = "serial"
	}
	if cfg.Transport != "serial" && cfg.Transport != "ble" || cfg.ConnectTimeout < 0 {
		return nil, fmt.Errorf("invalid transport or connection timeout")
	}
	if cfg.Transport == "ble" && cfg.BLEName == "" {
		cfg.BLEName = "IOS-Vlink"
	}
	if cfg.Timeout < 0 || cfg.InitialTimeout < 0 || cfg.RecoveryTimeout < 0 || cfg.Retries < 0 || cfg.Retries > 5 || open == nil {
		return nil, fmt.Errorf("invalid probe configuration; retries must be 0..5 and timeouts positive")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.InitialTimeout == 0 {
		cfg.InitialTimeout = 30 * time.Second
	}
	if cfg.RecoveryTimeout == 0 {
		cfg.RecoveryTimeout = 30 * time.Second
	}
	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = cfg.Timeout
		if cfg.Transport == "ble" {
			cfg.ConnectTimeout = 30 * time.Second
		}
	}
	return &Runner{config: cfg, open: open, wait: wait, now: time.Now}, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type linkError struct {
	err    error
	status elm327.Status
}

func (e *linkError) Error() string { return e.err.Error() }
func (e *linkError) Unwrap() error { return e.err }

func (r *Runner) Run(ctx context.Context) (Report, error) {
	report := Report{StartedAt: r.now().UTC(), Commands: []CommandLog{}}
	var session *elm327.Session
	var lastErr error
	needSync := false
	for attempt := 0; attempt <= r.config.Retries; attempt++ {
		resetReport(&report, r.config, attempt+1)
		if attempt > 0 {
			if err := r.wait(ctx, min(4*time.Second, 500*time.Millisecond*time.Duration(1<<min(attempt-1, 3)))); err != nil {
				lastErr = err
				break
			}
		}
		if session == nil {
			wire, err := r.openPort(ctx, &report)
			if err != nil {
				lastErr = err
				if ctx.Err() != nil {
					break
				}
				continue
			}
			session = elm327.NewSession(wire)
			if needSync {
				if err := r.recover(ctx, session, &report); err != nil {
					lastErr = errors.Join(err, session.Close())
					session = nil
					continue
				}
			}
		}
		lastErr = r.scan(ctx, session, &report)
		if lastErr == nil {
			lastErr = session.Close()
			session = nil
			break
		}
		if ctx.Err() != nil {
			break
		}
		var link *linkError
		if !errors.As(lastErr, &link) || attempt == r.config.Retries {
			break
		}
		if link.status == elm327.Timeout {
			if err := r.recover(ctx, session, &report); err == nil {
				continue
			}
		}
		lastErr = errors.Join(lastErr, session.Close())
		session = nil
		needSync = true
	}
	if session != nil {
		lastErr = errors.Join(lastErr, session.Close())
	}
	report.CompletedAt = r.now().UTC()
	if lastErr != nil {
		report.Status = "failed"
		report.Error = lastErr.Error()
		report.invalidate()
	}
	return report, lastErr
}

func (r *Runner) openPort(ctx context.Context, report *Report) (elm327.Transport, error) {
	ctx, cancel := context.WithTimeout(ctx, r.config.ConnectTimeout)
	defer cancel()
	started := r.now().UTC()
	wire, err := r.open(ctx)
	if err != nil {
		log := CommandLog{Attempt: report.Attempts, Action: "open", Source: report.Transport, StartedAt: started, CompletedAt: r.now().UTC(), Status: elm327.TransportError, Error: err.Error()}
		err = errors.Join(err, r.audit(report, log))
	}
	return wire, err
}

func (r *Runner) command(ctx context.Context, s *elm327.Session, report *Report, command string) (elm327.Response, error) {
	started := r.now()
	timeout := r.config.Timeout
	if command == "0100" {
		timeout = r.config.InitialTimeout
	}
	response, err := s.Command(ctx, command, timeout)
	completed := r.now()
	source := "adapter"
	if command[0] != 'A' {
		source = "ecu"
	}
	log := CommandLog{Attempt: report.Attempts, Action: "command", Command: command, Source: source, StartedAt: started.UTC(), CompletedAt: completed.UTC(), RawResponse: response.Raw, Status: response.Status}
	if err != nil {
		log.Error = err.Error()
	}
	r.commandTiming(&log, started, completed)
	if auditErr := r.audit(report, log); auditErr != nil {
		return response, auditErr
	}
	if err != nil {
		return response, &linkError{err: fmt.Errorf("command %s: %w", command, err), status: response.Status}
	}
	return response, nil
}

func (r *Runner) recover(ctx context.Context, s *elm327.Session, report *Report) error {
	started := r.now()
	response, err := s.Recover(ctx, r.config.RecoveryTimeout)
	completed := r.now()
	log := CommandLog{Attempt: report.Attempts, Action: "recovery", Source: "adapter", StartedAt: started.UTC(), CompletedAt: completed.UTC(), RawResponse: response.Raw, Status: response.Status}
	if err != nil {
		log.Error = err.Error()
	}
	r.commandTiming(&log, started, completed)
	return errors.Join(err, r.audit(report, log))
}

func (r *Runner) audit(report *Report, log CommandLog) error {
	if r.observer != nil {
		return r.observer(log)
	}
	report.Commands = append(report.Commands, log)
	return nil
}

func (r *Runner) commandTiming(log *CommandLog, started, completed time.Time) {
	if r.origin.IsZero() {
		return
	}
	log.StartedElapsedNS = started.Sub(r.origin).Nanoseconds()
	log.CompletedElapsedNS = completed.Sub(r.origin).Nanoseconds()
	log.LatencyNS = completed.Sub(started).Nanoseconds()
}
