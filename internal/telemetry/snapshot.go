package telemetry

import (
	"context"
	"time"
)

type Health string

const (
	Connecting   Health = "connecting"
	Connected    Health = "connected"
	Disconnected Health = "disconnected"
	Stale        Health = "stale"
	Simulator           = "simulator"
)

// Snapshot is immutable after publication. Nil means unavailable, never zero.
// Measurement units are part of the transport contract for future native clients.
type Snapshot struct {
	RPM        *float64  `json:"rpm"`
	SpeedKMH   *float64  `json:"speed_kmh"`
	CoolantC   *float64  `json:"coolant_c"`
	VoltageV   *float64  `json:"voltage_v"`
	Source     string    `json:"source"`
	CapturedAt time.Time `json:"captured_at"`
	ReceivedAt time.Time `json:"received_at"`
	Health     Health    `json:"health"`
	Message    string    `json:"message"`
}

// Source has one Read consumer. Start establishes the lifecycle; both its context
// and the context passed to Read must cancel pending work. Close is idempotent.
// Implementations must return immutable snapshots and honor cancellation.
type Source interface {
	Start(context.Context) error
	Read(context.Context) (Snapshot, error)
	Close() error
}

// At computes freshness without modifying a stored snapshot. Capture and receipt
// are checked independently so a freshly received old reading cannot appear live.
func (s Snapshot) At(now time.Time, maxAge time.Duration) Snapshot {
	if s.Health != Connected {
		return s
	}
	for _, timestamp := range []time.Time{s.CapturedAt, s.ReceivedAt} {
		age := now.Sub(timestamp)
		if timestamp.IsZero() || age < 0 || age >= maxAge {
			s.Health = Stale
			s.Message = "telemetry has expired"
			return s
		}
	}
	return s
}
