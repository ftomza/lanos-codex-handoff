package milstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

// These wire views avoid a cycle: mil owns the public payload types and uses
// milstore for persistence. Decode concrete telemetry fields before forwarding
// raw payloads so a malformed nested measurement cannot become evidence.
type readEvent struct {
	ID                 uint64                `json:"id"`
	Kind               string                `json:"kind"`
	State              string                `json:"state"`
	At                 time.Time             `json:"at"`
	Generation         uint64                `json:"generation"`
	PreviousCapturedAt time.Time             `json:"previous_captured_at"`
	Observation        *telemetry.MILReading `json:"observation"`
}

type readWindow struct {
	ID                 string    `json:"id"`
	TriggeredAt        time.Time `json:"triggered_at"`
	RequestedPreFrom   time.Time `json:"requested_pre_from"`
	RetainedFrom       time.Time `json:"retained_from"`
	PostUntil          time.Time `json:"post_until"`
	EndedAt            time.Time `json:"ended_at"`
	PreWindowRetained  bool      `json:"pre_window_retained"`
	PostWindowRetained bool      `json:"post_window_retained"`
	HadGaps            bool      `json:"had_gaps"`
	Reason             string    `json:"reason"`
}

type readFrame struct {
	EffectiveHealth telemetry.Health   `json:"effective_health"`
	At              time.Time          `json:"at"`
	Snapshot        telemetry.Snapshot `json:"snapshot"`
	SignalAges      map[string]struct {
		CapturedMS *int64 `json:"captured_ms"`
		ReceivedMS *int64 `json:"received_ms"`
		Applicable bool   `json:"applicable"`
	} `json:"signal_ages"`
	FuelContext json.RawMessage `json:"fuel_context"`
}

func validateReadPayload(e Entry) error {
	bad := fmt.Errorf("invalid %s payload", e.Kind)
	if e.Kind == "session_start" {
		if e.Incident != "" || len(e.Data) != 0 {
			return bad
		}
		return nil
	}
	data := bytes.TrimSpace(e.Data)
	if len(data) == 0 || data[0] != '{' {
		return bad
	}
	switch e.Kind {
	case "session_end":
		var v struct {
			Reason string `json:"reason"`
			Error  string `json:"error"`
		}
		if json.Unmarshal(data, &v) != nil || v.Reason == "" || e.Incident != "" {
			return bad
		}
	case "event":
		var v readEvent
		if json.Unmarshal(data, &v) != nil || v.At.IsZero() || v.ID == 0 || !validReadEvent(v.Kind, v.State) {
			return bad
		}
	case "incident_start", "incident_extend", "incident_end":
		var v readWindow
		if json.Unmarshal(data, &v) != nil || v.ID == "" || v.ID != e.Incident || v.TriggeredAt.IsZero() {
			return bad
		}
	case "pre", "post":
		var v readFrame
		if json.Unmarshal(data, &v) != nil || v.At.IsZero() || e.Incident == "" {
			return bad
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil || len(fields["snapshot"]) == 0 || bytes.Equal(bytes.TrimSpace(fields["snapshot"]), []byte("null")) {
			return bad
		}
	case "diagnostic":
		var v struct {
			Command string                `json:"command"`
			Reading *telemetry.ECUReading `json:"reading"`
			Poll    *telemetry.Poll       `json:"poll"`
		}
		if json.Unmarshal(data, &v) != nil || v.Reading == nil || v.Poll == nil || !validReadCommand(v.Command) {
			return bad
		}
	default:
		return fmt.Errorf("unknown row kind")
	}
	return nil
}

func validReadEvent(kind, state string) bool {
	switch kind {
	case "MIL_ON", "MIL_ALREADY_ON", "MIL_ON_AFTER_GAP":
		return state == "ON"
	case "MIL_OFF", "MIL_INITIAL_OFF", "MIL_OFF_AFTER_GAP":
		return state == "OFF"
	case "MIL_UNKNOWN":
		return state == "UNKNOWN"
	}
	return false
}

func validReadCommand(command string) bool {
	if command == "03" || command == "07" || command == "0A" {
		return true
	}
	if len(command) != 6 || command[:2] != "02" {
		return false
	}
	for _, c := range command[2:] {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}
