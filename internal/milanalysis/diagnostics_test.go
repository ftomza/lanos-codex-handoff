package milanalysis

import (
	"encoding/json"
	"slices"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestCorruptSuccessIsNotAnEmptyOrConfirmedDTCResult(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, command, pollStatus, detail string }{
		{"contradictory_status", "03", "invalid_response", `{"source":"ecu_mode03","status":"invalid_response","codes":["P0420"]}`},
		{"missing_detail", "03", "available", `null`},
		{"missing_codes", "03", "available", `{"source":"ecu_mode03","status":"available"}`},
		{"bad_code", "03", "available", `{"source":"ecu_mode03","status":"available","codes":["ZZZZZ"]}`},
		{"wrong_source", "03", "available", `{"source":"ecu_mode07","status":"available","codes":["P0420"]}`},
		{"wrong_frame", "020200", "available", `{"source":"ecu_mode02","status":"available","codes":["P0420"],"pid":"02","frame":1}`},
		{"wrong_pid", "020200", "available", `{"source":"ecu_mode02","status":"available","codes":["P0420"],"pid":"03","frame":0}`},
		{"wrong_numeric_unit", "020500", "available", `{"source":"ecu_mode02","status":"available","value":85,"unit":"V","pid":"05","frame":0}`},
		{"unexpected_numeric_codes", "020500", "available", `{"source":"ecu_mode02","status":"available","value":85,"unit":"degC","pid":"05","frame":0,"codes":["P0420"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := map[string]any{"command": tc.command, "reading": telemetry.ECUReading{Status: "available", CapturedAt: epoch, ReceivedAt: epoch, Detail: json.RawMessage(tc.detail)}, "poll": telemetry.Poll{Command: tc.command, Status: tc.pollStatus, At: epoch, Generation: 1}}
			r, err := Analyze(t.Context(), archive(t, []milstore.Entry{row(t, "diagnostic", "", epoch, d)}), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !r.Incomplete || len(r.Sessions[0].Diagnostics) != 0 || slices.Contains(codes(r), "p0420") {
				t.Fatal("trusted invalid decoded evidence", r)
			}
		})
	}
}

func TestUnsupportedNumericSignalCannotFeedRules(t *testing.T) {
	t.Parallel()
	f := sample(epoch, map[string]float64{"rpm": 900, "iat_c": -40})
	s := f.Snapshot.Signals["iat_c"]
	s.Support = "unsupported"
	f.Snapshot.Signals["iat_c"] = s
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f})), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(codes(r), "iat_out_of_range") {
		t.Fatal("unsupported signal used")
	}
}
