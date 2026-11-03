package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuelhistory"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type Config struct {
	MIL            *mil.Capture
	HistoryDir     string
	Recorder       *fuelhistory.Recorder
	Fuel           *fuel.Computer
	SourceName     string
	StaleAfter     time.Duration
	StreamInterval time.Duration
}

// Event adds server timing to the domain snapshot. Clients use elapsed local
// monotonic time rather than assuming their wall clock matches the server.
type Event struct {
	MILRecording *mil.CaptureStatus  `json:"mil_recording,omitempty"`
	Recording    *fuelhistory.Status `json:"fuel_recording,omitempty"`
	Fuel         *telemetry.Fuel     `json:"fuel,omitzero"`
	telemetry.Snapshot
	ServerTime   time.Time `json:"server_time"`
	StaleAfterMS int64     `json:"stale_after_ms"`
}

// Server keeps one immutable snapshot, independent of client count or speed.
type Server struct {
	milFailureLogged bool
	mu               sync.RWMutex
	latest           telemetry.Snapshot
	config           Config
}

func New(config Config) (*Server, error) {
	if config.HistoryDir != "" {
		if _, err := fuelhistory.List(config.HistoryDir); err != nil {
			return nil, err
		}
	}
	if config.StaleAfter < 0 || config.StreamInterval < 0 {
		return nil, fmt.Errorf("server timing must not be negative")
	}
	if config.StaleAfter == 0 {
		config.StaleAfter = 3 * time.Second
	}
	if config.StreamInterval == 0 {
		config.StreamInterval = 250 * time.Millisecond
	}
	if config.SourceName == "" {
		config.SourceName = "unknown"
	}
	return &Server{config: config, latest: telemetry.Snapshot{
		Source: config.SourceName, Health: telemetry.Connecting,
		Message: "waiting for the first source sample",
	}}, nil
}

func (s *Server) publish(snapshot telemetry.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest = snapshot
}

func (s *Server) snapshot() Event {
	s.mu.RLock()
	latest := s.latest
	s.mu.RUnlock()
	now := time.Now().UTC()
	event := Event{Snapshot: latest.At(now, s.config.StaleAfter), ServerTime: now, StaleAfterMS: s.config.StaleAfter.Milliseconds()}
	if s.config.Fuel != nil {
		view := s.config.Fuel.View(now)
		event.Fuel = &view
	}
	if s.config.Recorder != nil {
		status := s.config.Recorder.Status()
		event.Recording = &status
	}
	if s.config.MIL != nil {
		status := s.config.MIL.Status()
		event.MILRecording = &status
	}
	return event
}

// Run owns one source reader and closes the source on exit. Adapter reconnection
// belongs to the live source; transport subscribers never poll the source.
func (s *Server) Run(ctx context.Context, source telemetry.Source) (err error) {
	if s.config.Recorder != nil {
		defer func() { err = errors.Join(err, s.config.Recorder.Close()) }()
	}
	if s.config.Fuel != nil {
		defer func() { err = errors.Join(err, s.config.Fuel.Close()) }()
	}
	observed, err := s.observeMIL(source)
	if s.config.MIL != nil {
		defer func() { err = errors.Join(err, s.config.MIL.Close()) }()
	}
	if err != nil {
		return err
	}
	if err := source.Start(ctx); err != nil {
		return fmt.Errorf("start telemetry source: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	updates := make(chan sourceUpdate, 1)
	done := make(chan struct{})
	go func() { defer close(done); readSource(runCtx, source, updates) }()
	defer func() { cancel(); <-done; err = errors.Join(err, source.Close()) }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.tick(time.Now())
		case update := <-updates:
			if update.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				s.publish(telemetry.Snapshot{Source: s.snapshot().Source, Health: telemetry.Disconnected, Message: "telemetry source failed"})
				return fmt.Errorf("read telemetry source: %w", update.err)
			}
			if s.config.Fuel != nil {
				s.config.Fuel.Observe(time.Now(), update.snapshot)
			}
			if s.config.MIL != nil && !observed {
				s.config.MIL.Observe(update.snapshot)
			}
			s.publish(update.snapshot)
		}
	}
}

func (s *Server) observeMIL(source telemetry.Source) (bool, error) {
	if s.config.MIL == nil {
		return false, nil
	}
	observable, ok := source.(interface {
		SetObserver(func(telemetry.Snapshot)) error
	})
	if !ok {
		return false, nil
	}
	return true, observable.SetObserver(s.config.MIL.Observe)
}

func (s *Server) tick(now time.Time) {
	if s.config.Fuel != nil {
		s.config.Fuel.Tick(now)
	}
	if s.config.MIL == nil {
		return
	}
	s.config.MIL.Tick(now)
	status := s.config.MIL.Status()
	if status.State == "failed" && !s.milFailureLogged {
		log.Printf("mil recording failed: %s", status.Error)
		s.milFailureLogged = true
	}
}

type sourceUpdate struct {
	snapshot telemetry.Snapshot
	err      error
}

func readSource(ctx context.Context, source telemetry.Source, updates chan<- sourceUpdate) {
	for {
		snapshot, err := source.Read(ctx)
		select {
		case updates <- sourceUpdate{snapshot: snapshot, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}
