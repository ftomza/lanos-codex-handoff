package probe

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestPendingReadIsOptInAndAudited(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		enabled bool
		raw     string
		status  string
	}{{false, "470133\r>", ""}, {true, "4701330000\r>", "available"}, {true, "470000\r>", "available"}, {true, "NO DATA\r>", "temporarily_unavailable"}, {true, "430133\r>", "temporarily_unavailable"}, {true, "7F0711\r>", "temporarily_unavailable"}} {
		t.Run(tc.raw+tc.status, func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses["07"] = tc.raw
			r, err := New(Config{ReadPending: tc.enabled}, func(context.Context) (elm327.Transport, error) { return a, nil })
			if err != nil {
				t.Fatal(err)
			}
			report, err := r.Run(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(a.commands, "07") != tc.enabled {
				t.Fatal("unexpected pending schedule", a.commands)
			}
			if !tc.enabled {
				if report.PendingDTCs != nil {
					t.Fatal("default report changed")
				}
				return
			}
			p := report.PendingDTCs
			if p == nil || p.Source != "ecu_mode07" || p.Status != tc.status || p.RawResponse != tc.raw || p.CapturedAt.IsZero() {
				t.Fatal("missing pending evidence", p)
			}
			if a.commands[len(a.commands)-1] != "07" || report.Commands[len(report.Commands)-1].Command != "07" || !a.closed {
				t.Fatal("missing audit/close")
			}
			if tc.status != "available" {
				if report.Status != "partial" || p.Codes != nil {
					t.Fatal("unavailable became success", report.Status, p)
				}
				return
			}
			if tc.raw == "4701330000\r>" && !slices.Equal(p.Codes, []string{"P0133"}) {
				t.Fatal("lost code", p)
			}
			report.invalidate()
			if report.PendingDTCs.Status != "stale" || report.PendingDTCs.Codes != nil {
				t.Fatal("stale code exposed")
			}
			resetReport(&report, r.config, 2)
			if report.PendingDTCs.Status != "not_yet_tested" || report.PendingDTCs.Codes != nil {
				t.Fatal("retry retained code")
			}
		})
	}
}

func TestPendingReadRejectedForStreamsBeforeOpening(t *testing.T) {
	t.Parallel()
	opened := false
	r, err := New(Config{ReadPending: true}, func(context.Context) (elm327.Transport, error) { opened = true; return fixtureAdapter(t), nil })
	if err != nil {
		t.Fatal(err)
	}
	sink := func(record.Event) error { return nil }
	if _, err := r.Record(t.Context(), time.Second, sink); err == nil {
		t.Fatal("record accepted finite option")
	}
	if _, err := r.Stream(t.Context(), sink); err == nil {
		t.Fatal("stream accepted finite option")
	}
	if opened {
		t.Fatal("hardware opened")
	}
}
