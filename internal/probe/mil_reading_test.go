package probe

import (
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func TestRecordingDiagnosticReadingStrictServicesAndFreezeContext(t *testing.T) {
	t.Parallel()
	cases := []struct{ command, raw, status, source string }{
		{"03", "430133", "available", "ecu_mode03"},
		{"07", "470133", "available", "ecu_mode07"},
		{"0A", "4A0133", "available", "ecu_mode0a"},
		{"07", "430133", "invalid_response", "ecu_mode07"},
		{"03", "7F0311", "unsupported", "ecu_mode03"},
		{"07", "7F0722", "temporarily_unavailable", "ecu_mode07"},
		{"0A", "7F0311", "invalid_response", "ecu_mode0a"},
		{"020000", "42000040000000", "capability", "ecu_mode02"},
		{"020200", "4202000133", "available", "ecu_mode02"},
		{"020C00", "420C001AF8", "available", "ecu_mode02"},
		{"020300", "4203000100", "raw_only", "ecu_mode02"},
		{"020C00", "410C1AF8", "invalid_response", "ecu_mode02"},
		{"020C00", "420C011AF8", "invalid_response", "ecu_mode02"},
		{"020000", "7F0212", "unsupported", "ecu_mode02"},
	}
	for _, tc := range cases {
		t.Run(tc.command+tc.raw, func(t *testing.T) {
			t.Parallel()
			raw := tc.raw + "\r>"
			response := elm327.Parse(tc.command, raw)
			got := recordingReading(CommandLog{Action: "command", Command: tc.command, RawResponse: raw, Status: response.Status})
			if got == nil || got.Status != tc.status || got.Source != tc.source {
				t.Fatalf("reading=%+v want=%+v", got, tc)
			}
			if tc.command == "020000" && tc.status == "capability" && (len(got.Supported) != 1 || got.Supported[0] != "02") {
				t.Fatalf("bitmap=%+v", got)
			}
			if tc.source == "ecu_mode02" && (got.Frame == nil || *got.Frame != 0) {
				t.Fatalf("missing frame context: %+v", got)
			}
		})
	}
}

func TestRepeatedDTCUnavailableClearsOldCodes(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	r, _ := recordingRunner(t, a, 0)
	session := elm327.NewSession(a)
	var report Report
	resetReport(&report, Config{}, 1)
	result := &report.StoredDTCs
	a.responses["03"] = "430133\r>"
	if err := r.readDTC(t.Context(), session, &report, "03", result, elm327.DecodeDTC); err != nil {
		t.Fatal(err)
	}
	a.responses["03"] = "7F0311\r>"
	if err := r.readDTC(t.Context(), session, &report, "03", result, elm327.DecodeDTC); err != nil {
		t.Fatal(err)
	}
	if result.Status != "unsupported" || len(result.Codes) != 0 {
		t.Fatalf("stale codes survived: %+v", result)
	}
}
