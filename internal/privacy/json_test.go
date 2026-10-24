package privacy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONRedactsNestedStringsWithoutLosingIntegers(t *testing.T) {
	t.Parallel()
	const vin = "1HGCM82633A004352"
	value := map[string]any{"elapsed_ns": int64(98765432101234567), "raw": "49 02 01 31 32\r>", "errors": []any{map[string]any{"error": vin}}}
	data, err := JSON(value, true)
	if err != nil || strings.Contains(string(data), vin) || strings.Contains(string(data), "49 02") || !strings.Contains(string(data), "98765432101234567") {
		t.Fatalf("redaction/integer precision: %s %v", data, err)
	}
	if !json.Valid(data) || data[len(data)-1] != '\n' {
		t.Fatalf("not a JSON line: %s", data)
	}
	data, err = JSON(value, false)
	if err != nil || !strings.Contains(string(data), vin) {
		t.Fatalf("explicit unredacted: %s %v", data, err)
	}
}
