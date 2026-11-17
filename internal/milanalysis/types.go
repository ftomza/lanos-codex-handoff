// Package milanalysis examines recorded evidence; it never opens a vehicle transport.
package milanalysis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

const maxSessions = 128
const maxItems = 256
const maxFindings = 128
const maxFrames = 1024
const maxExamples = 8

type Options struct{ Session, Incident string }

func (o Options) Validate() error {
	if o.Session != "" {
		_, err := hex.DecodeString(o.Session)
		if err != nil || len(o.Session) != 32 || o.Session != strings.ToLower(o.Session) {
			return fmt.Errorf("invalid MIL session id")
		}
	}
	if o.Incident != "" {
		if o.Session == "" || len(o.Incident) != 25 || !strings.HasPrefix(o.Incident, "incident-") {
			return fmt.Errorf("incident selection requires session and a valid incident id")
		}
		_, err := hex.DecodeString(o.Incident[9:])
		if err != nil || strings.ToLower(o.Incident) != o.Incident {
			return fmt.Errorf("invalid incident id")
		}
	}
	return nil
}

type Reference struct {
	File       string    `json:"file"`
	Line       int       `json:"line"`
	Sequence   uint64    `json:"sequence"`
	Session    string    `json:"session_id"`
	Incident   string    `json:"incident,omitempty"`
	At         time.Time `json:"at"`
	Generation uint64    `json:"generation"`
}

type Evidence struct {
	Reference
	Signal     string    `json:"signal,omitempty"`
	CapturedAt time.Time `json:"captured_at,omitzero"`
	Value      *float64  `json:"value,omitempty"`
	Unit       string    `json:"unit,omitempty"`
	Detail     string    `json:"detail,omitempty"`
}

type Finding struct {
	Code            string     `json:"code"`
	Symptom         string     `json:"symptom"`
	Confidence      string     `json:"confidence"`
	Count           int        `json:"count"`
	FirstObservedAt time.Time  `json:"first_observed_at"`
	LastObservedAt  time.Time  `json:"last_observed_at"`
	Supporting      []Evidence `json:"supporting_evidence"`
	Contradicting   []Evidence `json:"contradicting_evidence"`
	Missing         []string   `json:"missing_data"`
	NextCheck       []string   `json:"next_check"`
}

type RuleCheck struct {
	Code   string `json:"code"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Event struct {
	Reference
	Event mil.Event `json:"event"`
}

type Detail struct {
	Error     string             `json:"error,omitempty"`
	Extra     map[string]float64 `json:"extra,omitempty"`
	Codes     []string           `json:"codes"`
	Value     *float64           `json:"value"`
	Unit      string             `json:"unit,omitempty"`
	Status    string             `json:"status,omitempty"`
	Source    string             `json:"source,omitempty"`
	Frame     *int               `json:"frame,omitempty"`
	PID       string             `json:"pid,omitempty"`
	Supported []string           `json:"supported,omitempty"`
}

type Reading struct {
	Status      string    `json:"status"`
	CapturedAt  time.Time `json:"captured_at"`
	ReceivedAt  time.Time `json:"received_at"`
	RawResponse string    `json:"raw_response"`
	Detail      Detail    `json:"detail"`
}

type Diagnostic struct {
	Reference
	Command string         `json:"command"`
	Reading Reading        `json:"reading"`
	Poll    telemetry.Poll `json:"poll"`
}

type Incident struct {
	ID         string      `json:"id"`
	Window     mil.Window  `json:"window"`
	Closed     bool        `json:"closed"`
	FrameCount int         `json:"frame_count"`
	Incomplete bool        `json:"incomplete"`
	Findings   []Finding   `json:"findings"`
	Checks     []RuleCheck `json:"rule_checks"`
}

type Session struct {
	ID          string       `json:"id"`
	Source      string       `json:"source"`
	StartedAt   time.Time    `json:"started_at"`
	EndedAt     time.Time    `json:"ended_at,omitzero"`
	Closed      bool         `json:"closed"`
	EndReason   string       `json:"end_reason,omitempty"`
	Error       string       `json:"error,omitempty"`
	Incomplete  bool         `json:"incomplete"`
	Summary     string       `json:"summary"`
	Events      []Event      `json:"events"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	Incidents   []Incident   `json:"incidents"`
	Findings    []Finding    `json:"findings"`
}

type Report struct {
	Version     int                  `json:"version"`
	Source      string               `json:"source"`
	Incomplete  bool                 `json:"incomplete"`
	Sessions    []Session            `json:"sessions"`
	Issues      []milstore.ReadIssue `json:"issues"`
	Limitations []string             `json:"limitations"`
}

func payload(data json.RawMessage, out any) error {
	if len(data) == 0 || string(data) == "null" {
		return fmt.Errorf("missing payload")
	}
	return json.Unmarshal(data, out)
}

func reference(e milstore.ReadEntry) Reference {
	return Reference{File: e.File, Line: e.Line, Sequence: e.Sequence, Session: e.Session, Incident: e.Incident, At: e.At}
}

func excerpt(s string) string {
	if len(s) > 512 {
		return s[:512] + " [truncated]"
	}
	return s
}
