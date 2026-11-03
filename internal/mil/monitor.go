// Package mil detects host-observed MIL changes without inferring ECU onset.
package mil

import (
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type Event struct {
	ID                 uint64                `json:"id"`
	Kind               string                `json:"kind"`
	State              string                `json:"state"`
	At                 time.Time             `json:"at"`
	Generation         uint64                `json:"generation"`
	PreviousCapturedAt time.Time             `json:"previous_captured_at,omitzero"`
	Observation        *telemetry.MILReading `json:"observation,omitempty"`
}

func (e Event) Trigger() bool {
	return e.Kind == "MIL_ON" || e.Kind == "MIL_ALREADY_ON" || e.Kind == "MIL_ON_AFTER_GAP"
}

// MILMonitor is owned by one capture manager; it has no transport or goroutine.
type MILMonitor struct {
	latest     telemetry.Snapshot
	staleAfter time.Duration
	state      string
	seen       bool
	generation uint64
	capturedAt time.Time
	sequence   uint64
}

func NewMonitor(staleAfter time.Duration) *MILMonitor {
	return &MILMonitor{staleAfter: staleAfter, state: "UNKNOWN"}
}

func (m *MILMonitor) Observe(now time.Time, snapshot telemetry.Snapshot) []Event {
	events := make([]Event, 0, 2)
	if m.seen && m.state != "UNKNOWN" && snapshot.Generation != m.generation {
		events = append(events, m.unknown(now))
	}
	m.latest = snapshot.Clone()
	m.generation = snapshot.Generation
	effective := snapshot.At(now, m.staleAfter)
	r := effective.MIL
	if effective.Health != telemetry.Connected || r == nil || r.Status != "available" || r.On == nil {
		if m.state != "UNKNOWN" {
			events = append(events, m.unknown(now))
		}
		return events
	}
	// Only a new MIL response can re-establish continuity after loss. Core PID
	// recovery or a conflicting duplicate must not turn old MIL into a new event.
	if m.seen && !r.CapturedAt.After(m.capturedAt) {
		return events
	}
	state := "OFF"
	if *r.On {
		state = "ON"
	}
	if m.state == state {
		m.capturedAt = maxTime(m.capturedAt, r.CapturedAt)
		return events
	}
	kind := "MIL_" + state
	if !m.seen {
		kind = "MIL_INITIAL_OFF"
		if state == "ON" {
			kind = "MIL_ALREADY_ON"
		}
	} else if m.state == "UNKNOWN" {
		kind += "_AFTER_GAP"
	}
	previous := m.capturedAt
	if m.state == "UNKNOWN" {
		previous = time.Time{}
	}
	m.state, m.seen, m.capturedAt = state, true, r.CapturedAt
	m.sequence++
	return append(events, Event{ID: m.sequence, Kind: kind, State: state, At: now.UTC(), Generation: m.generation, PreviousCapturedAt: previous, Observation: effective.MIL})
}

func (m *MILMonitor) Tick(now time.Time) []Event { return m.Observe(now, m.latest) }

func (m *MILMonitor) unknown(now time.Time) Event {
	m.state = "UNKNOWN"
	m.sequence++
	effective := m.latest.At(now, m.staleAfter)
	return Event{ID: m.sequence, Kind: "MIL_UNKNOWN", State: "UNKNOWN", At: now.UTC(), Generation: m.generation, Observation: effective.MIL}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
