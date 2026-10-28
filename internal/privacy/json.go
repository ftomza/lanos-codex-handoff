// Package privacy redacts recognizable unsolicited VINs from exported diagnostics.
package privacy

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

var vinPattern = regexp.MustCompile(`\b[A-HJ-NPR-Z0-9]{17}\b`)

func VIN(s string) string {
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\r' || r == '\n' }) {
		compact := strings.ToUpper(strings.Join(strings.Fields(line), ""))
		if strings.HasPrefix(compact, "4902") {
			return "[unsolicited Mode 09 VIN response redacted]"
		}
	}
	return vinPattern.ReplaceAllString(s, "[VIN REDACTED]")
}

func sanitize(value any) any {
	switch v := value.(type) {
	case string:
		return VIN(v)
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

// JSON returns one complete line. UseNumber preserves monotonic nanosecond fields.
func JSON(value any, redact bool) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if !redact {
		return append(data, '\n'), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	data, err = json.Marshal(sanitize(value))
	return append(data, '\n'), err
}
