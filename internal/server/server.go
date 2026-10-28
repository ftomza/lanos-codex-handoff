package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type Config struct {
	Fuel           *fuel.Computer
	SourceName     string
	StaleAfter     time.Duration
	StreamInterval time.Duration
}

// Event adds server timing to the domain snapshot. Clients use elapsed local
// monotonic time rather than assuming their wall clock matches the server.
type Event struct {
	Fuel *telemetry.Fuel `json:"fuel,omitzero"`
	telemetry.Snapshot
	ServerTime   time.Time `json:"server_time"`
	StaleAfterMS int64     `json:"stale_after_ms"`
}

// Server keeps one immutable snapshot, independent of client count or speed.
type Server struct {
	mu     sync.RWMutex
	latest telemetry.Snapshot
	config Config
}

func New(config Config) (*Server, error) {
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
	return event
}

// Run owns one source reader and closes the source on exit. Adapter reconnection
// belongs to the live source; transport subscribers never poll the source.
func (s *Server) Run(ctx context.Context, source telemetry.Source) (err error) {
	if s.config.Fuel != nil {
		defer func() { err = errors.Join(err, s.config.Fuel.Close()) }()
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
			if s.config.Fuel != nil {
				s.config.Fuel.Tick(time.Now())
			}
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
			s.publish(update.snapshot)
		}
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
