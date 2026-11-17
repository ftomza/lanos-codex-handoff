package probe

import (
	"strings"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func isDiagnosticCommand(command string) bool {
	return command == "03" || command == "07" || command == "0A" || strings.HasPrefix(command, "02")
}

// Decode historical services before itemPID: a frame number is not a Mode01 PID
// suffix. Source/frame keep freeze values explicitly separate from live signals.
func recordingDiagnostic(log CommandLog) (*RecordingReading, bool) {
	if !isDiagnosticCommand(log.Command) {
		return nil, false
	}
	result := &RecordingReading{Status: "temporarily_unavailable", Error: log.Error}
	service := byte(3)
	switch log.Command {
	case "03":
		result.Source = "ecu_mode03"
	case "07":
		result.Source = "ecu_mode07"
		service = 7
	case "0A":
		result.Source = "ecu_mode0a"
		service = 10
	default:
		frame := 0
		result.Source, result.Frame = "ecu_mode02", &frame
		service = 2
	}
	if log.Error != "" {
		return result, true
	}
	response := elm327.Parse(log.Command, log.RawResponse)
	if status := elm327.DiagnosticStatus(response, service); status != "" {
		result.Status = status
		return result, true
	}
	var err error
	switch service {
	case 3:
		result.Codes, err = elm327.DecodeDTC(response)
	case 7:
		result.Codes, err = elm327.DecodePendingDTC(response)
	case 10:
		result.Codes, err = elm327.DecodePermanentDTC(response)
	case 2:
		return recordingFreeze(log.Command, response, result), true
	}
	result.Status = "available"
	if err != nil {
		result.Status, result.Error = "invalid_response", err.Error()
	}
	return result, true
}

func recordingFreeze(command string, response elm327.Response, result *RecordingReading) *RecordingReading {
	if len(command) != 6 || command[4:] != "00" {
		result.Status = "invalid_response"
		return result
	}
	pid := itemPID(command[:4])
	expected, ok := elm327.FreezeFrameCommand(pid)
	if !ok || command != expected {
		result.Status = "invalid_response"
		return result
	}
	result.PID = elm327.HexPID(pid)
	result.Status = "available"
	var err error
	switch pid {
	case 0:
		var pids []byte
		pids, err = elm327.DecodeFreezeSupported(response)
		if err == nil {
			result.Status = "capability"
			result.Supported = make([]string, 0, len(pids))
			for _, p := range pids {
				result.Supported = append(result.Supported, elm327.HexPID(p))
			}
		}
	case 2:
		result.Codes, err = elm327.DecodeFreezeDTC(response)
	default:
		result.Measurement, err = elm327.DecodeFreezePID(pid, response)
		if err == nil && result.Value == nil {
			result.Status = "raw_only"
		}
	}
	if err != nil {
		result.Status, result.Error = "invalid_response", err.Error()
	}
	return result
}
