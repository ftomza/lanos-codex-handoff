package sim

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

var (
	ErrNotStarted = errors.New("simulator has not started")
	ErrStarted    = errors.New("simulator has already started")
	ErrClosed     = errors.New("simulator is closed")
)

// Simulator samples on demand, with no background goroutine or growing queue.
type Simulator struct {
	mu            sync.Mutex
	interval      time.Duration
	start         time.Time
	next          time.Time
	lifecycleDone <-chan struct{}
	stop          chan struct{}
	closed        bool
}

func New(interval time.Duration) (*Simulator, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("simulator interval must be positive")
	}
	return &Simulator{interval: interval, stop: make(chan struct{})}, nil
}

func (s *Simulator) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if !s.start.IsZero() {
		return ErrStarted
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.start = time.Now().UTC()
	s.lifecycleDone = ctx.Done()
	return nil
}

func (s *Simulator) Read(ctx context.Context) (telemetry.Snapshot, error) {
	s.mu.Lock()
	start, next, done, closed := s.start, s.next, s.lifecycleDone, s.closed
	s.mu.Unlock()
	if closed {
		return telemetry.Snapshot{}, ErrClosed
	}
	if start.IsZero() {
		return telemetry.Snapshot{}, ErrNotStarted
	}
	if err := ctx.Err(); err != nil {
		return telemetry.Snapshot{}, err
	}
	select {
	case <-done:
		return telemetry.Snapshot{}, context.Canceled
	default:
	}
	timer := time.NewTimer(max(0, time.Until(next)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return telemetry.Snapshot{}, ctx.Err()
	case <-done:
		return telemetry.Snapshot{}, context.Canceled
	case <-s.stop:
		return telemetry.Snapshot{}, ErrClosed
	case <-timer.C:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return telemetry.Snapshot{}, ErrClosed
	}
	now := time.Now().UTC()
	s.next = now.Add(s.interval)
	return Sample(start, now), nil
}

func (s *Simulator) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	close(s.stop)
	return nil
}
