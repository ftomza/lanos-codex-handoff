package elm327

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestAgentPhysicalBLEFixture(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/vgate-ble-2026-10-24.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Observations []struct{ Command, Raw, Log string }
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Observations) != 42 {
		t.Fatalf("unexpected physical exchange count: %d", len(fixture.Observations))
	}
	for _, observation := range fixture.Observations {
		response := Parse(observation.Command, observation.Raw)
		if response.Status != OK || observation.Log == "" {
			t.Fatalf("physical frame: %+v", observation)
		}
		switch observation.Command {
		case "0120":
			pids, err := DecodeSupported(0x20, response)
			if err != nil || !slices.Equal(pids, []byte{0x21, 0x22}) {
				t.Fatalf("real continuation: %v %v", pids, err)
			}
		case "010C":
			measurement, err := DecodePID(0x0C, response)
			if err != nil || measurement.Value == nil || *measurement.Value < 800 || *measurement.Value > 1000 {
				t.Fatalf("physical RPM: %+v %v", measurement, err)
			}
		case "0105":
			measurement, err := DecodePID(5, response)
			if err != nil || measurement.Value == nil || *measurement.Value < 50 || *measurement.Value > 52 {
				t.Fatalf("physical coolant: %+v %v", measurement, err)
			}
		case "ATRV":
			if !strings.HasSuffix(response.Lines[0], "V") {
				t.Fatalf("lost adapter units: %+v", response)
			}
		case "03":
			codes, err := DecodeDTC(response)
			if err != nil || len(codes) != 0 {
				t.Fatalf("physical stored DTC response: %v %v", codes, err)
			}
		case "0101":
			measurement, err := DecodePID(1, response)
			if err != nil || measurement.MIL == nil || *measurement.MIL || measurement.DTCCount == nil || *measurement.DTCCount != 0 {
				t.Fatalf("physical MIL response: %+v %v", measurement, err)
			}
		}
	}
}
