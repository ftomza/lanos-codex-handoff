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
	Live                = "live"
)

// Snapshot is immutable after publication. Nil means unavailable, never zero.
// Measurement units are part of the transport contract for future native clients.
type Snapshot struct {
	Diagnostics  Diagnostics       `json:"diagnostics,omitzero"`
	Signals      map[string]Signal `json:"signals,omitempty"`
	Transport    string            `json:"transport,omitzero"`
	SourceDetail string            `json:"source_detail,omitzero"`
	Adapter      string            `json:"adapter,omitzero"`
	Protocol     string            `json:"protocol,omitzero"`
	RPM          *float64          `json:"rpm"`
	SpeedKMH     *float64          `json:"speed_kmh"`
	CoolantC     *float64          `json:"coolant_c"`
	VoltageV     *float64          `json:"voltage_v"`
	Source       string            `json:"source"`
	CapturedAt   time.Time         `json:"captured_at"`
	ReceivedAt   time.Time         `json:"received_at"`
	Health       Health            `json:"health"`
	Message      string            `json:"message"`
}

// Diagnostics are bounded counters for the whole live run, across reconnects.
// Recoveries counts lifecycle/recovery events, not successful new connections.
type Diagnostics struct {
	Commands   uint64 `json:"commands"`
	Errors     uint64 `json:"errors"`
	Gaps       uint64 `json:"gaps"`
	Recoveries uint64 `json:"recoveries"`
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
	if s.Health == Connected && !freshTimes(now, maxAge, s.CapturedAt, s.ReceivedAt) {
		s.Health, s.Message = Stale, "telemetry has expired"
	}
	if len(s.Signals) == 0 {
		return s
	}
	return s.signalsAt(now)
}
