package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestPartialFailureStillExportsAudit(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	cause := errors.New("transport disconnected")
	err := run(t.Context(), []string{"--port=/dev/cu.fake", "--json"}, &out, &stderr, dependencies{probe: func(context.Context, probe.Config) (probe.Report, error) {
		return probe.Report{Status: "failed", Error: cause.Error(), Commands: []probe.CommandLog{{RawResponse: "410C00"}}}, cause
	}})
	if !errors.Is(err, cause) || !strings.Contains(out.String(), "410C00") || !strings.Contains(out.String(), "failed") {
		t.Fatalf("audit lost: %v %s", err, out.String())
	}
}

func TestRedactionPolicy(t *testing.T) {
	t.Parallel()
	const vin = "1HGCM82633A004352"
	for _, raw := range []string{vin + "\r>", "49 02 01 31 48 47\r02 43 4D 38\r>"} {
		redacted := redactVIN(raw)
		if redacted == raw || strings.Contains(redacted, vin) {
			t.Fatalf("not redacted: %q", redacted)
		}
	}
	r := probe.Report{Adapter: vin}
	data, err := encodeReport(r, false)
	if err != nil || !strings.Contains(string(data), vin) {
		t.Fatalf("explicit raw export: %s %v", data, err)
	}
	if got := redactVIN("4100BE3EB811\r410C4902\r>"); got != "4100BE3EB811\r410C4902\r>" {
		t.Fatal("redaction damaged ordinary PID data")
	}
}

func TestHumanOutputShowsAdapterVoltageAndUntestedDTCStatus(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	r := probe.Report{AdapterVoltage: probe.AdapterVoltage{Value: new(10.9), Status: "available", Unit: "V"}, StoredDTCs: probe.DTCResult{Status: "not_yet_tested"},
		Capabilities: []probe.Capability{{PID: "01", Support: "supported", Status: "available", Measurement: elm327.Measurement{MIL: new(false), DTCCount: new(0)}}}}
	if err := showReport(&out, r, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "10.9 V") || !strings.Contains(out.String(), "ATRV") || !strings.Contains(out.String(), "not_yet_tested") {
		t.Fatalf("missing reading/provenance: %s", out.String())
	}
	if !strings.Contains(out.String(), "mil_on=false") || !strings.Contains(out.String(), "stored_dtc_count=0") {
		t.Fatalf("MIL was displayed as unknown: %s", out.String())
	}
}
