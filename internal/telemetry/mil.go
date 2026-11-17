package telemetry

import (
	"encoding/json"
	"maps"
	"slices"
	"time"
)

// MILReading uses host response completion and receipt timestamps, not ECU onset.
type MILReading struct {
	On           *bool           `json:"on"`
	DTCCount     *int            `json:"dtc_count"`
	Status       string          `json:"status"`
	RawResponse  string          `json:"raw_response"`
	Readiness    json.RawMessage `json:"readiness,omitempty"`
	CapturedAt   time.Time       `json:"captured_at"`
	ReceivedAt   time.Time       `json:"received_at"`
	StaleAfterMS int64           `json:"stale_after_ms"`
	AgeMS        *int64          `json:"age_ms"`
}

// ECUReading contains stored diagnostic evidence; it never updates live signals.
type ECUReading struct {
	Status      string          `json:"status"`
	RawResponse string          `json:"raw_response"`
	CapturedAt  time.Time       `json:"captured_at"`
	ReceivedAt  time.Time       `json:"received_at"`
	Detail      json.RawMessage `json:"detail,omitempty"`
}

type Poll struct {
	Command    string    `json:"command"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	LatencyNS  int64     `json:"latency_ns"`
	At         time.Time `json:"at"`
	Generation uint64    `json:"generation"`
}

func (s Snapshot) cloneDiagnostics() Snapshot {
	if s.MIL != nil {
		m := *s.MIL
		if m.On != nil {
			m.On = new(*m.On)
		}
		if m.DTCCount != nil {
			m.DTCCount = new(*m.DTCCount)
		}
		if m.AgeMS != nil {
			m.AgeMS = new(*m.AgeMS)
		}
		m.Readiness = slices.Clone(m.Readiness)
		s.MIL = &m
	}
	s.ECUReadings = maps.Clone(s.ECUReadings)
	for k, r := range s.ECUReadings {
		r.Detail = slices.Clone(r.Detail)
		s.ECUReadings[k] = r
	}
	if s.Poll != nil {
		s.Poll = new(*s.Poll)
	}
	return s
}

func (s Snapshot) milAt(now time.Time) Snapshot {
	if s.MIL == nil {
		return s
	}
	s = s.cloneDiagnostics()
	m := s.MIL
	if !m.CapturedAt.IsZero() {
		m.AgeMS = new(now.Sub(m.CapturedAt).Milliseconds())
	}
	if s.Health != Connected || !freshTimes(now, time.Duration(m.StaleAfterMS)*time.Millisecond, m.CapturedAt, m.ReceivedAt) {
		m.On, m.DTCCount = nil, nil
		if m.Status == "available" {
			m.Status = "stale"
		}
	}
	return s
}
