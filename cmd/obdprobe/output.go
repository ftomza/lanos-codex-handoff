package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

var vinPattern = regexp.MustCompile(`\b[A-HJ-NPR-Z0-9]{17}\b`)

func redactVIN(s string) string {
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }) {
		compact := strings.ToUpper(strings.Join(strings.Fields(line), ""))
		if strings.HasPrefix(compact, "4902") {
			return "[unsolicited Mode 09 VIN response redacted]"
		}
	}
	return vinPattern.ReplaceAllString(s, "[VIN REDACTED]")
}

// Sanitize string fields recursively so unsolicited VIN text cannot escape via
// a decoder error or a warning while raw responses themselves are redacted.
func sanitize(value any) any {
	switch v := value.(type) {
	case string:
		return redactVIN(v)
	case []any:
		for i := range v {
			v[i] = sanitize(v[i])
		}
	case map[string]any:
		for key, item := range v {
			v[key] = sanitize(item)
		}
	}
	return value
}

func encodeReport(report probe.Report, redact bool) ([]byte, error) {
	data, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	if !redact {
		return append(data, '\n'), nil
	}
	var value any
	// UseNumber preserves integer timestamps/values while redacting string fields.
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	data, err = json.Marshal(sanitize(value))
	return append(data, '\n'), err
}

func showPorts(w io.Writer, asJSON bool, list func() ([]string, error)) error {
	ports, err := list()
	if err != nil {
		return err
	}
	if ports == nil {
		ports = []string{}
	}
	if asJSON {
		return json.NewEncoder(w).Encode(struct {
			Ports    []string `json:"ports"`
			Guidance string   `json:"guidance"`
		}{ports, portGuidance})
	}
	if len(ports) == 0 {
		_, err = fmt.Fprintln(w, "No macOS serial ports found.\n"+portGuidance)
		return err
	}
	for _, port := range ports {
		if _, err := fmt.Fprintln(w, port); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(w, "Select the Vgate port explicitly. Enumeration does not verify connectivity or ECU compatibility.")
	return err
}

func showReport(w io.Writer, report probe.Report, redact bool) error {
	clean := func(s string) string {
		if redact {
			return redactVIN(s)
		}
		return s
	}
	if _, err := fmt.Fprintf(w, "Read-only probe: %s; attempts=%d; source=%s\n", report.Status, report.Attempts, report.Source); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Device model: %s; model_source=%s; transport=%s (adapter identity comes from ATI)\n", clean(report.Device.Model), report.Device.ModelSource, report.Transport); err != nil {
		return err
	}
	for _, record := range report.Commands {
		if _, err := fmt.Fprintf(w, "%s attempt=%d %s %s status=%s raw=%q error=%q\n", record.CompletedAt.Format("2006-01-02T15:04:05.000Z07:00"), record.Attempt, record.Action, record.Command, record.Status, clean(record.RawResponse), clean(record.Error)); err != nil {
			return err
		}
	}
	if report.Protocol != nil {
		if _, err := fmt.Fprintf(w, "Protocol %s: %s (ECU model is unconfirmed)\n", report.Protocol.Code, report.Protocol.Name); err != nil {
			return err
		}
	}
	for _, c := range report.Capabilities {
		if c.Support == "not_yet_tested" || c.Support == "unsupported" {
			continue
		}
		value := "unknown"
		if c.Value != nil {
			value = fmt.Sprintf("%.3f %s", *c.Value, c.Unit)
		}
		if _, err := fmt.Fprintf(w, "PID %s %-30s support=%s status=%s value=%s\n", c.PID, c.Name, c.Support, c.Status, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "Stored Mode 03 DTCs: status=%s codes=%v (pending/permanent codes not queried)\n", report.StoredDTCs.Status, report.StoredDTCs.Codes); err != nil {
		return err
	}
	for _, warning := range report.Warnings {
		if _, err := fmt.Fprintln(w, "Warning:", clean(warning)); err != nil {
			return err
		}
	}
	return nil
}
