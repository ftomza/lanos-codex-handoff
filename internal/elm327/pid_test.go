package elm327

import "testing"

func TestPriorityPIDDecoders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		pid  byte
		raw  string
		want float64
		unit string
	}{
		{4, "410480\r>", 128.0 * 100 / 255, "%"}, {6, "410680\r>", 0, "%"},
		{7, "410700\r>", -100, "%"}, {0x0B, "410B65\r>", 101, "kPa"},
		{0x0F, "410F50\r>", 40, "degC"}, {0x10, "411001F4\r>", 5, "g/s"},
		{0x11, "4111FF\r>", 100, "%"}, {0x14, "41146480\r>", 0.5, "V"},
		{0x42, "414236B0\r>", 14, "V"}, {0x5E, "415E0064\r>", 5, "L/h"},
	} {
		t.Run(HexPID(tc.pid), func(t *testing.T) {
			t.Parallel()
			m, err := DecodePID(tc.pid, Parse("01"+HexPID(tc.pid), tc.raw))
			if err != nil || m.Value == nil || *m.Value != tc.want || m.Unit != tc.unit {
				t.Fatalf("measurement: %+v %v", m, err)
			}
		})
	}
	m, err := DecodePID(0x14, Parse("0114", "411464FF\r>"))
	if err != nil || len(m.Extra) != 0 {
		t.Fatal("unavailable oxygen fuel trim fabricated")
	}
	m, err = DecodePID(0xAA, Parse("01AA", "41AA12\r>"))
	if err != nil || m.Value != nil {
		t.Fatal("unknown PID was given an invented decoder")
	}
}

func TestProtocolMalformed(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"A5\r6\r>", "AG\r>", "A55\r>", "?\r>", "D\r>"} {
		if _, err := DecodeProtocol(Parse("ATDPN", raw)); err == nil {
			t.Fatalf("accepted protocol %q", raw)
		}
	}
	p, err := DecodeProtocol(Parse("ATDPN", "5\r>"))
	if err != nil || p.Automatic || p.Number != 5 {
		t.Fatalf("manual protocol: %+v %v", p, err)
	}
}
