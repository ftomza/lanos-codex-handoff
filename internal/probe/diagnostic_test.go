package probe

import (
	"context"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestExtendedFiniteReadingsAndAudit(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0A"] = "4A01330000\r>"
	a.responses["020000"] = "42000040180000\r>"
	a.responses["020200"] = "4202000133\r>"
	a.responses["020C00"] = "420C001000\r>"
	a.responses["020D00"] = "420D0019\r>"
	r, err := New(Config{ReadPermanent: true, ReadFreezeFrame: true}, func(context.Context) (elm327.Transport, error) { return a, nil })
	if err != nil {
		t.Fatal(err)
	}
	report, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p := report.PermanentDTCs
	if p == nil || p.Status != "available" || !slices.Equal(p.Codes, []string{"P0133"}) || p.CapturedAt.IsZero() {
		t.Fatal(p)
	}
	f := report.FreezeFrame
	if f == nil || f.Status != "available" || f.Bitmap == nil || len(f.Readings) != 3 || f.Readings[1].Value == nil || *f.Readings[1].Value != 1024 || f.Readings[1].Unit != "rpm" {
		t.Fatal(f)
	}
	wanted := []string{"0A", "020000", "020200", "020C00", "020D00"}
	if !slices.Equal(a.commands[len(a.commands)-5:], wanted) {
		t.Fatal(a.commands)
	}
	report.invalidate()
	if report.PermanentDTCs.Status != "stale" || report.PermanentDTCs.Codes != nil || report.FreezeFrame.Status != "stale" || report.FreezeFrame.Readings[1].Value != nil {
		t.Fatal("stale data exposed")
	}
	resetReport(&report, r.config, 2)
	if report.PermanentDTCs.Status != "not_yet_tested" || len(report.FreezeFrame.Readings) != 0 {
		t.Fatal("retry retained data")
	}
}

func TestFreezeFrameNeverReadsUnadvertisedPIDs(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"42000000000000\r>", "NO DATA\r>", "7F0211\r>", "420001FFFFFFFF\r>"} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["020000"] = raw
			r, _ := New(Config{ReadFreezeFrame: true}, func(context.Context) (elm327.Transport, error) { return a, nil })
			report, err := r.Run(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if a.commands[len(a.commands)-1] != "020000" || len(report.FreezeFrame.Readings) != 0 {
				t.Fatal(a.commands, report.FreezeFrame)
			}
		})
	}
}

func TestPermanentResponseStatus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ raw, status string }{{"7F0A11\r>", "unsupported"}, {"7F0A12\r>", "unsupported"}, {"7F0A22\r>", "temporarily_unavailable"}, {"NO DATA\r>", "temporarily_unavailable"}, {"4A01\r>", "invalid_response"}, {"430133\r>", "invalid_response"}} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["0A"] = tc.raw
			r, _ := New(Config{ReadPermanent: true}, func(context.Context) (elm327.Transport, error) { return a, nil })
			report, err := r.Run(t.Context())
			if err != nil || report.PermanentDTCs.Status != tc.status || report.PermanentDTCs.RawResponse != tc.raw {
				t.Fatal(report.PermanentDTCs, err)
			}
		})
	}
}

func TestExtendedReadsRejectedBeforeStreamOpen(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{{ReadPermanent: true}, {ReadFreezeFrame: true}, {Headers: true}} {
		opened := false
		r, _ := New(cfg, func(context.Context) (elm327.Transport, error) { opened = true; return fixtureAdapter(t), nil })
		sink := func(record.Event) error { return nil }
		if _, err := r.Record(t.Context(), time.Second, sink); err == nil {
			t.Fatal("record accepted")
		}
		if _, err := r.Stream(t.Context(), sink); err == nil {
			t.Fatal("stream accepted")
		}
		if opened {
			t.Fatal("opened")
		}
	}
}

func TestFiniteHeadersNormalizeAndAuditResponder(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["ATH1"] = "OK\r>"
	a.responses["0A"] = "83F1107F0A11\r>"
	for command, raw := range a.responses {
		if command[0] == 'A' || command == "0A" {
			continue
		}
		parsed := elm327.Parse(command, raw)
		if parsed.Status != elm327.OK {
			continue
		}
		lines := []string{}
		for _, line := range parsed.Lines {
			data, err := hex.DecodeString(strings.ReplaceAll(line, " ", ""))
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, fmt.Sprintf("%02XF110%s", 0x80|len(data), hex.EncodeToString(data)))
		}
		a.responses[command] = strings.Join(lines, "\r") + "\r>"
	}
	r, _ := New(Config{Headers: true, ReadPermanent: true}, func(context.Context) (elm327.Transport, error) { return a, nil })
	report, err := r.Run(t.Context())
	if err != nil || report.PermanentDTCs.Status != "unsupported" {
		t.Fatal(report.PermanentDTCs, err)
	}
	if slices.Index(a.commands, "ATH1") < 0 || slices.Index(a.commands, "ATH1") > slices.Index(a.commands, "0100") {
		t.Fatal(a.commands)
	}
	if c := capability(t, report, 12); c.Value == nil || c.Status != "available" {
		t.Fatal(c)
	}
	if a.commands[len(a.commands)-1] != "ATH0" {
		t.Fatal("headers not restored", a.commands)
	}
	last := report.Commands[len(report.Commands)-2]
	if len(last.Frames) != 1 || last.Frames[0].Source != "10" || len(report.PermanentDTCs.Frames) != 1 {
		t.Fatal("lost header evidence", last)
	}
	baseline, wire := runFixture(t, false)
	if baseline.PermanentDTCs != nil || baseline.FreezeFrame != nil || slices.Contains(wire.commands, "ATH1") || !slices.Equal(wire.commands, fixtureCommands(t)) {
		t.Fatal("default changed", wire.commands)
	}
}

func fixtureCommands(t *testing.T) []string {
	t.Helper()
	// These are the original finite serial baseline commands in execution order.
	return []string{"ATZ", "ATE0", "ATL0", "ATS0", "ATH0", "ATI", "ATSP0", "0100", "ATDPN", "ATRV", "010C", "0105", "010D", "0101", "03"}
}

func TestFreezeReadHelperRequiresValidatedAdvertisement(t *testing.T) {
	t.Parallel()
	for _, bitmap := range []*Bitmap{nil, {Status: "invalid_response", Supported: []string{"0C"}}, {Status: "available", Supported: []string{"0D"}}} {
		a := fixtureAdapter(t)
		r, _ := New(Config{}, func(context.Context) (elm327.Transport, error) { return a, nil })
		report := Report{}
		result := FreezeFrameResult{Bitmap: bitmap}
		if err := r.readFreezePID(t.Context(), elm327.NewSession(a), &report, &result, 12); err == nil || len(a.commands) != 0 {
			t.Fatal("unadvertised context queried", a.commands, err)
		}
	}
}

func TestHeaderCleanupFailureIsVisibleAndTimeoutDoesNotWrite(t *testing.T) {
	t.Parallel()
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprint(timeout), func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["ATH1"] = "OK\r>"
			a.responses["ATH0"] = "NO DATA\r>"
			cfg := Config{Transport: "ble", Headers: true, ReadPermanent: true, Timeout: 5 * time.Millisecond}
			if timeout {
				a.failCommand = "0A"
			} else {
				a.responses["0A"] = "83F1104A0000\r>"
			}
			r, _ := New(cfg, func(context.Context) (elm327.Transport, error) { return a, nil })
			report, err := r.Run(t.Context())
			if err == nil || report.Status != "failed" || !a.closed {
				t.Fatal("cleanup error hidden", report.Status, err)
			}
			last := a.commands[len(a.commands)-1]
			if timeout && last != "0A" || !timeout && last != "ATH0" {
				t.Fatal("unsafe cleanup", a.commands)
			}
		})
	}
}
