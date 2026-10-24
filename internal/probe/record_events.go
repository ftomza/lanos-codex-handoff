package probe

import (
	"runtime/debug"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

type RecordingSummary struct {
	Reason       string `json:"reason"`
	Commands     int64  `json:"commands"`
	Samples      int64  `json:"samples"`
	Errors       int64  `json:"errors"`
	Gaps         int    `json:"gaps"`
	Segments     int    `json:"segments"`
	DurationNS   int64  `json:"duration_ns"`
	PollingNS    int64  `json:"polling_ns"`
	UnobservedNS int64  `json:"unobserved_ns"`
}

type RecordingCapabilities struct {
	Report        Report   `json:"report"`
	MissingInputs []string `json:"missing_inputs"`
}

type RecordingReading struct {
	Status string   `json:"status"`
	PID    string   `json:"pid,omitzero"`
	Error  string   `json:"error,omitzero"`
	Codes  []string `json:"codes,omitzero"`
	elm327.Measurement
}

type RecordingCommand struct {
	CommandLog
	StartedElapsedNS   int64             `json:"started_elapsed_ns"`
	CompletedElapsedNS int64             `json:"completed_elapsed_ns"`
	LatencyNS          int64             `json:"latency_ns"`
	Result             *RecordingReading `json:"result,omitzero"`
}

func recordingReading(log CommandLog) *RecordingReading {
	if log.Action != "command" || !(strings.HasPrefix(log.Command, "01") || log.Command == "03" || log.Command == "ATRV") {
		return nil
	}
	result := &RecordingReading{Status: "temporarily_unavailable"}
	if strings.HasPrefix(log.Command, "01") {
		result.PID = log.Command[2:]
	}
	if log.Status != elm327.OK || log.Error != "" {
		result.Error = log.Error
		return result
	}
	response := elm327.Parse(log.Command, log.RawResponse)
	var err error
	switch {
	case log.Command == "ATRV":
		var value float64
		value, err = decodeVoltage(response)
		if err == nil {
			result.Value, result.Unit = &value, "V"
		}
	case log.Command == "03":
		result.Codes, err = elm327.DecodeDTC(response)
	case itemPID(log.Command)%32 == 0:
		_, err = elm327.DecodeSupported(itemPID(log.Command), response)
		if err == nil {
			result.Status = "capability"
			return result
		}
	default:
		result.Measurement, err = elm327.DecodePID(itemPID(log.Command), response)
		if err == nil && result.Value == nil && result.MIL == nil {
			result.Status = "raw_only"
			return result
		}
	}
	if err != nil {
		result.Status, result.Error = "invalid_response", err.Error()
		return result
	}
	result.Status = "available"
	return result
}

func (s *recording) emit(kind string, data any) error {
	if s.outputErr != nil {
		return s.outputErr
	}
	now := s.r.now()
	s.outputErr = s.sink(record.Event{Type: kind, Timestamp: now.UTC(), ElapsedNS: now.Sub(s.started).Nanoseconds(), Segment: s.summary.Segments, Data: data})
	return s.outputErr
}

func (s *recording) observe(log CommandLog) error {
	if log.Action == "recovery" {
		if log.Error != "" {
			s.summary.Errors++
		}
		return s.emit("recovery", log)
	}
	if log.Action == "open" {
		s.summary.Errors++
		return s.emit("recovery", log)
	}
	s.summary.Commands++
	reading := recordingReading(log)
	if log.Error != "" || log.Status != elm327.OK || reading != nil && (reading.Status == "invalid_response" || reading.Status == "temporarily_unavailable") {
		s.summary.Errors++
	}
	if reading != nil && (reading.Status == "available" || reading.Status == "raw_only") && log.Command != "03" {
		s.summary.Samples++
	}
	return s.emit("command", RecordingCommand{CommandLog: log, StartedElapsedNS: log.StartedElapsedNS, CompletedElapsedNS: log.CompletedElapsedNS, LatencyNS: log.LatencyNS, Result: reading})
}

func recordingMetadata(cfg Config, duration time.Duration) map[string]any {
	revision := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				revision = setting.Value
			}
		}
	}
	return map[string]any{
		"timezone": "Europe/Moscow", "build_revision": revision, "config": cfg,
		"duration_ns": duration.Nanoseconds(), "core_pids": []string{"0C", "0B", "0D"},
		"core_target_interval_ns": int64(time.Second), "supporting_schedule": supportingItems(),
		"engine_displacement_l": 1.5, "engine_displacement_source": "owner_reported",
		"limitations": "host timestamps, sequential readings; no measured fuel flow or calibrated fuel estimate; gaps must not be integrated",
	}
}
