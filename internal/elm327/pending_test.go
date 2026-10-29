package elm327

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func TestPendingDTCStrictModeAndPadding(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw   string
		codes []string
		bad   bool
	}{
		{"47 01 33 00 00 00 00\r>", []string{"P0133"}, false},
		{"47 00 00 00 00 00 00\r>", []string{}, false},
		{"47 01 33\r47 01 33\r>", []string{"P0133"}, false},
		{"43 01 33\r>", nil, true}, {"47 01\r>", nil, true},
		{"47 00 00 01 33\r>", nil, true}, {"NO DATA\r>", nil, true},
		{"7F 07 11\r>", nil, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			codes, err := DecodePendingDTC(Parse("07", tc.raw))
			if (err != nil) != tc.bad || !tc.bad && !slices.Equal(codes, tc.codes) {
				t.Fatalf("codes=%v err=%v", codes, err)
			}
		})
	}
	if !SafeCommand("07") {
		t.Fatal("pending read blocked")
	}
	for _, command := range []string{"04", "08", "0A", "0902", "ATSH8110F1", "07\r04"} {
		if SafeCommand(command) {
			t.Fatal("unsafe or out-of-scope command allowed", command)
		}
	}
	if _, err := DecodeDTC(Parse("03", "470133\r>")); err == nil {
		t.Fatal("pending became stored")
	}
}

func TestPhysicalMILWithEmptyStoredAndPendingReplies(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/vgate-pending-2026-10-29.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Exchanges []struct{ Command, Raw string }
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, exchange := range fixture.Exchanges {
		response := Parse(exchange.Command, exchange.Raw)
		switch exchange.Command {
		case "0101":
			m, err := DecodePID(1, response)
			if err != nil || m.MIL == nil || !*m.MIL || m.DTCCount == nil || *m.DTCCount != 0 {
				t.Fatal("MIL evidence changed", m, err)
			}
		case "03":
			codes, err := DecodeDTC(response)
			if err != nil || len(codes) != 0 {
				t.Fatal("stored evidence changed", codes, err)
			}
		case "07":
			codes, err := DecodePendingDTC(response)
			if err != nil || len(codes) != 0 {
				t.Fatal("pending evidence changed", codes, err)
			}
		}
	}
}
