package elm327

import (
	"slices"
	"testing"
)

// All new readiness, permanent, freeze-frame and header cases are synthetic.
func TestReadinessMonitorBits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw, ignition         string
		name                  string
		supported, incomplete bool
	}{
		{"41018007E1A1\r>", "spark", "catalyst", true, true},
		{"4101000F8101\r>", "compression", "NMHC catalyst", true, true},
	} {
		t.Run(tc.ignition, func(t *testing.T) {
			t.Parallel()
			m, err := DecodePID(1, Parse("0101", tc.raw))
			if err != nil || m.Readiness == nil {
				t.Fatal(m, err)
			}
			r := m.Readiness
			if r.Ignition != tc.ignition || len(r.Raw) != 4 {
				t.Fatal(r)
			}
			found := false
			for _, v := range r.Monitors {
				if v.Name == tc.name {
					found = true
					if v.Supported != tc.supported || v.Incomplete != tc.incomplete {
						t.Fatal(v)
					}
				}
			}
			if !found {
				t.Fatal(r)
			}
		})
	}
}

func TestPermanentDTCAndFiniteAllowlist(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw string
		bad bool
	}{{"4A01330000\r>", false}, {"4A0000\r>", false}, {"430133\r>", true}, {"4A01\r>", true}, {"4A00000133\r>", true}, {"7F0A11\r>", true}} {
		codes, err := DecodePermanentDTC(Parse("0A", tc.raw))
		if (err != nil) != tc.bad {
			t.Fatal(codes, err, tc)
		}
	}
	for _, c := range []string{"0A", "ATH1", "020000", "020200", "020C00", "021F00"} {
		if !SafeCommand(c) {
			t.Fatal("blocked", c)
		}
	}
	for _, c := range []string{"04", "08", "0902", "02", "020201", "022000", "020A00", "ATSH8110F1", "ATH2", "0a", "020C00\r04"} {
		if SafeCommand(c) {
			t.Fatal("allowed", c)
		}
	}
}

func TestFreezeFrameStrictContext(t *testing.T) {
	t.Parallel()
	pids, err := DecodeFreezeSupported(Parse("020000", "42000040180000\r>"))
	if err != nil || !slices.Equal(pids, []byte{2, 12, 13}) {
		t.Fatal(pids, err)
	}
	m, err := DecodeFreezePID(12, Parse("020C00", "420C001000\r>"))
	if err != nil || m.Value == nil || *m.Value != 1024 || m.Unit != "rpm" {
		t.Fatal(m, err)
	}
	codes, err := DecodeFreezeDTC(Parse("020200", "4202000133\r>"))
	if err != nil || !slices.Equal(codes, []string{"P0133"}) {
		t.Fatal(codes, err)
	}
	for _, raw := range []string{"420C011000\r>", "410C1000\r>", "420C0010\r>"} {
		if _, err := DecodeFreezePID(12, Parse("020C00", raw)); err == nil {
			t.Fatal(raw)
		}
	}
}

func TestFreezeFuelSystemLength(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"420300\r>", "42030000\r>", "420300000000\r>"} {
		if _, err := DecodeFreezePID(3, Parse("020300", raw)); err == nil {
			t.Fatalf("accepted malformed synthetic PID03: %q", raw)
		}
	}
	m, err := DecodeFreezePID(3, Parse("020300", "4203000200\r>"))
	if err != nil || m.Value != nil {
		t.Fatalf("valid raw-only fuel-system context: %+v %v", m, err)
	}
}

func TestKWPHeadersValidateBeforeNormalization(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"834F104A0000\r>", "84F1104A0000\r>", "83F1104A000000\r>", "80F110034A0000\r>", "7E8034A0000\r>", "4A0000\r>"} {
		r, err := NormalizeKWP(Parse("0A", raw))
		if err == nil || r.Raw != raw || r.Status != Malformed {
			t.Fatal(raw, r, err)
		}
	}
	for _, raw := range []string{"83F1104A0000\r>", "83F1104A0000CE\r>"} {
		r, err := NormalizeKWP(Parse("0A", raw))
		if err != nil || len(r.Frames) != 1 || r.Frames[0].Source != "10" || r.Raw != raw {
			t.Fatal(r, err)
		}
		if _, err := DecodePermanentDTC(r); err != nil {
			t.Fatal(err)
		}
	}
	r, err := NormalizeKWP(Parse("0A", "83F1107F0A111E\r>"))
	if err != nil || r.Status != ECURejected || DiagnosticStatus(r, 10) != "unsupported" {
		t.Fatal(r, err)
	}
}

func TestDiagnosticNegativeAvailability(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ raw, want string }{{"7F0A11\r>", "unsupported"}, {"7F0A12\r>", "unsupported"}, {"7F0A22\r>", "temporarily_unavailable"}, {"NO DATA\r>", "temporarily_unavailable"}, {"7F0711\r>", "invalid_response"}, {"7F0A\r>", "invalid_response"}} {
		if got := DiagnosticStatus(Parse("0A", tc.raw), 10); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestHeaderResponderIdentityAndAmbiguity(t *testing.T) {
	t.Parallel()
	raw := "84F110410C1000\r84F111410C2000\r>"
	r, err := NormalizeKWP(Parse("010C", raw))
	if err != nil || len(r.Frames) != 2 || r.Frames[0].Source != "10" || r.Frames[1].Source != "11" {
		t.Fatal(r, err)
	}
	if _, err := DecodePID(12, r); err == nil {
		t.Fatal("different responder values silently selected")
	}
	for _, raw := range []string{"83F1004A0000\r>", "83F1F14A0000\r>", "83F1FF4A0000\r>"} {
		if _, err := NormalizeKWP(Parse("0A", raw)); err == nil {
			t.Fatal("invalid responder", raw)
		}
	}
}
