package elm327

import "strings"

type Status string

const (
	OK             Status = "ok"
	NoData         Status = "no_data"
	ECUUnavailable Status = "ecu_unavailable"
	ECURejected    Status = "ecu_rejected"
	AdapterError   Status = "adapter_error"
	Stopped        Status = "stopped"
	Malformed      Status = "malformed_response"
	Timeout        Status = "timeout"
	TransportError Status = "transport_error"
)

type Response struct {
	Raw    string   `json:"raw_response"`
	Lines  []string `json:"lines"`
	Status Status   `json:"status"`
}

func compact(s string) string { return strings.ToUpper(strings.Join(strings.Fields(s), "")) }

// Parse normalizes echo and adapter chatter but retains the original bytes.
// Prompt completion is mandatory even when a plausible payload arrived earlier.
func Parse(command, raw string) Response {
	r := Response{Raw: raw, Status: OK, Lines: make([]string, 0, 4)}
	before, after, found := strings.Cut(raw, ">")
	if !found || strings.TrimSpace(after) != "" {
		r.Status = Malformed
		return r
	}
	for _, line := range strings.FieldsFunc(before, func(r rune) bool { return r == '\r' || r == '\n' }) {
		line = strings.TrimSpace(line)
		if line == "" || compact(line) == compact(command) {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "SEARCHING...") {
			line = strings.TrimSpace(line[len("SEARCHING..."):])
			if line == "" {
				continue
			}
		}
		if strings.HasPrefix(compact(line), "BUSINIT:") && strings.HasSuffix(compact(line), "OK") {
			continue
		}
		r.Lines = append(r.Lines, line)
		if status := lineStatus(line); status != OK {
			r.Status = status
		}
	}
	if len(r.Lines) == 0 {
		r.Status = Malformed
	}
	return r
}

func lineStatus(line string) Status {
	s := compact(line)
	switch {
	case s == "NODATA":
		return NoData
	case strings.Contains(s, "UNABLETOCONNECT"), strings.HasPrefix(s, "BUSINIT:") && strings.Contains(s, "ERROR"):
		return ECUUnavailable
	case s == "STOPPED":
		return Stopped
	case s == "?", strings.Contains(s, "ERROR"), strings.Contains(s, "BUFFERFULL"), strings.Contains(s, "<DATAERROR"):
		return AdapterError
	case strings.HasPrefix(s, "7F"):
		return ECURejected
	default:
		return OK
	}
}
