package elm327

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestRealVgateSession(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/vgate-2026-10-24.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Exchanges []struct{ Command, Raw string }
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	responses := make(map[string]Response, len(fixture.Exchanges))
	for _, exchange := range fixture.Exchanges {
		r := Parse(exchange.Command, exchange.Raw)
		if r.Status != OK {
			t.Fatalf("%s: %+v", exchange.Command, r)
		}
		responses[exchange.Command] = r
	}
	p, err := DecodeProtocol(responses["ATDPN"])
	if err != nil || !p.Automatic || p.Number != 5 || p.Name != "ISO 14230-4 KWP2000 (fast init)" {
		t.Fatalf("protocol: %+v, %v", p, err)
	}
	pids, err := DecodeSupported(0, responses["0100"])
	want := []byte{1, 3, 4, 5, 6, 7, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x11, 0x13, 0x14, 0x15, 0x1C, 0x20}
	if err != nil || !reflect.DeepEqual(pids, want) {
		t.Fatalf("supported: %X, %v", pids, err)
	}
	for pid, want := range map[byte]float64{0x0C: 0, 0x05: 78, 0x0D: 0} {
		m, err := DecodePID(pid, responses["01"+HexPID(pid)])
		if err != nil || m.Value == nil || *m.Value != want {
			t.Fatalf("pid %X: %+v, %v", pid, m, err)
		}
	}
	codes, err := DecodeDTC(responses["03"])
	if err != nil || len(codes) != 0 {
		t.Fatalf("DTC: %v, %v", codes, err)
	}
}

func TestDecodeMalformedAndMultipleResponses(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"410C00\r>", "410D12\r>", "410CZZZZ\r>", "410C0000\r410C1AF8\r>", "NO DATA\r>", "7F0112\r>"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := DecodePID(0x0C, Parse("010C", raw)); err == nil {
				t.Fatalf("accepted %q", raw)
			}
		})
	}
	m, err := DecodePID(0x0C, Parse("010C", "41 0C 1A F8\r41 0C 1A F8\r>"))
	if err != nil || m.Value == nil || *m.Value != 1726 {
		t.Fatalf("RPM: %+v, %v", m, err)
	}
	if _, err := DecodeSupported(0, Parse("0100", "4100BE3EB8\r>")); err == nil {
		t.Fatal("accepted partial bitmap")
	}
	if _, err := DecodeSupported(0xE0, Parse("01E0", "41E000000001\r>")); err == nil {
		t.Fatal("accepted overflowing PID 100")
	}
}

func TestDTCAndMIL(t *testing.T) {
	t.Parallel()
	codes, err := DecodeDTC(Parse("03", "4301335234C123\r4301330000\r>"))
	if err != nil || !reflect.DeepEqual(codes, []string{"P0133", "C1234", "U0123"}) {
		t.Fatalf("DTC: %v %v", codes, err)
	}
	for _, raw := range []string{"4301\r>", "4300000133\r>", "NO DATA\r>", "7F0311\r>"} {
		if _, err := DecodeDTC(Parse("03", raw)); err == nil {
			t.Fatalf("accepted DTC %q", raw)
		}
	}
	m, err := DecodePID(1, Parse("0101", "410182000000\r>"))
	if err != nil || m.MIL == nil || !*m.MIL || m.DTCCount == nil || *m.DTCCount != 2 {
		t.Fatalf("MIL: %+v %v", m, err)
	}
}

func TestResponseStatuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw  string
		want Status
	}{
		{"010C\rSEARCHING...\r410C0000\r>", OK}, {"NO DATA\r>", NoData},
		{"UNABLE TO CONNECT\r>", ECUUnavailable}, {"BUS INIT: ERROR\r>", ECUUnavailable},
		{"STOPPED\r>", Stopped}, {"?\r>", AdapterError}, {"CAN ERROR\r>", AdapterError},
		{"410C0000", Malformed}, {">", Malformed},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			if got := Parse("010C", tc.raw).Status; got != tc.want {
				t.Fatalf("%q: %s, want %s", tc.raw, got, tc.want)
			}
		})
	}
}
