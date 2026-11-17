package milanalysis

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

func validateDiagnostic(v diagnosticPayload, at time.Time) (Detail, error) {
	var d Detail
	if !diagnosticCommand(v.Command) || v.Poll.Command != v.Command || v.Reading.CapturedAt.IsZero() || v.Reading.ReceivedAt.IsZero() || v.Reading.CapturedAt.After(at) || v.Reading.ReceivedAt.After(at) {
		return d, fmt.Errorf("invalid diagnostic reading metadata")
	}
	if !slices.Contains([]string{"available", "capability", "raw_only", "unsupported", "temporarily_unavailable", "unavailable", "invalid_response"}, v.Reading.Status) {
		return d, fmt.Errorf("unknown diagnostic reading status")
	}
	if len(v.Reading.Detail) > 0 {
		if err := payload(v.Reading.Detail, &d); err != nil {
			return d, err
		}
	}
	if len(d.Codes) > 64 || len(d.Supported) > 32 || len(d.PID) > 2 || len(d.Source) > 64 || len(d.Status) > 64 || len(d.Unit) > 32 || len(d.Extra) > 16 {
		return d, fmt.Errorf("diagnostic detail exceeds limits")
	}
	for _, c := range d.Codes {
		if !validDTC(c) {
			return d, fmt.Errorf("invalid diagnostic code")
		}
	}
	for _, p := range d.Supported {
		if !hexByte(p) {
			return d, fmt.Errorf("invalid supported PID")
		}
	}
	d.Error = excerpt(d.Error)
	source := "ecu_mode" + strings.ToLower(v.Command)
	if len(v.Command) == 6 {
		source = "ecu_mode02"
	}
	if d.Source != "" && d.Source != source {
		return d, fmt.Errorf("diagnostic source does not match command")
	}
	success := v.Reading.Status == "available" || v.Reading.Status == "capability" || v.Reading.Status == "raw_only"
	if !success {
		return d, nil
	}
	if d.Source != source || d.Status != v.Reading.Status || v.Poll.Status != v.Reading.Status || d.Error != "" || v.Poll.Error != "" {
		return d, fmt.Errorf("inconsistent successful diagnostic result")
	}
	if len(v.Command) == 2 {
		if v.Reading.Status != "available" || d.Codes == nil || d.PID != "" || d.Frame != nil || d.Value != nil || d.Supported != nil {
			return d, fmt.Errorf("missing decoded DTC list or wrong service detail")
		}
		return d, nil
	}
	if v.Command[4:] != "00" || d.Frame == nil || *d.Frame != 0 || d.PID != v.Command[2:4] {
		return d, fmt.Errorf("freeze detail does not match requested frame and PID")
	}
	if d.PID != "02" && d.Codes != nil {
		return d, fmt.Errorf("codes on non-code freeze PID")
	}
	if d.PID != "00" && d.Supported != nil {
		return d, fmt.Errorf("bitmap on non-capability freeze PID")
	}
	switch d.PID {
	case "00":
		if v.Reading.Status != "capability" || d.Value != nil {
			return d, fmt.Errorf("freeze bitmap lacks capability status")
		}
	case "02":
		if d.Codes == nil || v.Reading.Status != "available" || d.Value != nil {
			return d, fmt.Errorf("freeze DTC list missing")
		}
	default:
		unit := freezeUnit(d.PID)
		if unit != "" && (d.Value == nil || d.Unit != unit || v.Reading.Status != "available") {
			return d, fmt.Errorf("freeze numeric value or unit does not match PID")
		}
		if unit == "" && (v.Reading.Status != "raw_only" || d.Value != nil) {
			return d, fmt.Errorf("undecoded freeze PID must remain raw_only")
		}
	}
	return d, nil
}

func validDTC(c string) bool {
	return len(c) == 5 && strings.ContainsRune("PCBU", rune(c[0])) && c[1] >= '0' && c[1] <= '3' && hexDigits(c[2:])
}

func hexByte(s string) bool { return len(s) == 2 && hexDigits(s) }
func hexDigits(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func freezeUnit(pid string) string {
	switch pid {
	case "04", "06", "07", "08", "09", "11":
		return "%"
	case "05", "0F":
		return "degC"
	case "0B":
		return "kPa"
	case "0C":
		return "rpm"
	case "0D":
		return "km/h"
	case "0E":
		return "deg"
	case "10":
		return "g/s"
	case "1F":
		return "s"
	}
	if pid >= "14" && pid <= "1B" {
		return "V"
	}
	return ""
}
