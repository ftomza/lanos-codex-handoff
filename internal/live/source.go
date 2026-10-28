// Package live adapts the shared read-only ELM stream to the telemetry Source contract.
package live

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

var (
	ErrNotStarted = errors.New("live source has not started")
	ErrStarted    = errors.New("live source has already started")
	ErrClosed     = errors.New("live source is closed")
)

type streamFunc func(context.Context, func(record.Event) error) (probe.RecordingSummary, error)

type Source struct {
	mu                      sync.Mutex
	stream                  streamFunc
	now                     func() time.Time
	latest                  telemetry.Snapshot
	version, delivered      uint64
	changed                 chan struct{}
	workerDone              chan struct{}
	lifecycleDone           <-chan struct{}
	cancel                  context.CancelFunc
	workerErr               error
	started, closed, active bool
}

func New(cfg probe.Config, open func(context.Context) (elm327.Transport, error)) (*Source, error) {
	if cfg.Transport == "" {
		cfg.Transport = "serial"
	}
	runner, err := probe.New(cfg, open)
	if err != nil {
		return nil, err
	}
	return newSource(runner.Stream, cfg.Transport), nil
}

func newSource(stream streamFunc, transport string) *Source {
	s := &Source{stream: stream, now: time.Now, changed: make(chan struct{}), workerDone: make(chan struct{}), version: 1}
	s.latest = telemetry.Snapshot{Source: telemetry.Live, Transport: "ble_gatt", SourceDetail: "elm327_ble", Health: telemetry.Connecting, Message: "Подключение к адаптеру", Signals: initialSignals()}
	if transport == "serial" {
		s.latest.Transport, s.latest.SourceDetail = "serial_spp", "elm327_serial"
	}
	return s
}

func (s *Source) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.started {
		return ErrStarted
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.stream == nil {
		return fmt.Errorf("live source requires a stream")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel, s.lifecycleDone, s.started = cancel, runCtx.Done(), true
	go s.run(runCtx)
	return nil
}

func (s *Source) run(ctx context.Context) {
	defer close(s.workerDone)
	_, err := s.stream(ctx, s.consume)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workerErr = err
	if s.closed {
		return
	}
	message := "Источник остановлен. Перезапустите приборку."
	if err != nil {
		message = "Источник недоступен: " + privacy.VIN(err.Error()) + ". Перезапустите приборку."
	}
	s.clear(telemetry.Disconnected, message, false)
	s.publish()
}

func (s *Source) Read(ctx context.Context) (telemetry.Snapshot, error) {
	for {
		if err := ctx.Err(); err != nil {
			return telemetry.Snapshot{}, err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return telemetry.Snapshot{}, ErrClosed
		}
		if !s.started {
			s.mu.Unlock()
			return telemetry.Snapshot{}, ErrNotStarted
		}
		select {
		case <-s.lifecycleDone:
			s.mu.Unlock()
			return telemetry.Snapshot{}, context.Canceled
		default:
		}
		if s.version != s.delivered {
			s.delivered = s.version
			value := s.latest.Clone()
			s.mu.Unlock()
			return value, nil
		}
		changed, done := s.changed, s.lifecycleDone
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return telemetry.Snapshot{}, ctx.Err()
		case <-done:
			return telemetry.Snapshot{}, context.Canceled
		case <-changed:
		}
	}
}

func (s *Source) Close() error {
	s.mu.Lock()
	s.closed = true
	cancel, started := s.cancel, s.started
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if started {
		<-s.workerDone
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workerErr
}

// Called under mu. No queue grows when readers are slower than the adapter.
func (s *Source) publish() { s.version++; close(s.changed); s.changed = make(chan struct{}) }
