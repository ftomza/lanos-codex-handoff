package probe

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func TestAdvertisedContinuationIsTheOnlyWayToFuelLevel(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0120"] = "412000020000\r>"
	a.responses["012F"] = "412F80\r>"
	r, err := New(Config{ScanSupported: true}, func(context.Context) (elm327.Transport, error) { return a, nil })
	if err != nil {
		t.Fatal(err)
	}
	report, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := capability(t, report, 0x2F)
	if c.Support != "supported" || c.Value == nil || *c.Value < 50 || *c.Value > 51 || !slices.Contains(a.commands, "012F") || slices.Contains(a.commands, "0140") {
		t.Fatalf("capability gating: %+v, %v", c, a.commands)
	}
}

func TestFailedRecoveryNeverBlindlyWritesOnReopen(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand = "010C"
	b := fixtureAdapter(t)
	opens := 0
	r, err := New(Config{Timeout: time.Millisecond, RecoveryTimeout: time.Millisecond, Retries: 1}, func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return a, nil
		}
		return b, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.wait = func(context.Context, time.Duration) error { return nil }
	report, err := r.Run(t.Context())
	if err == nil || report.Status != "failed" || len(b.commands) != 0 {
		t.Fatalf("unsafe reopen: %v, %s, %v", err, report.Status, b.commands)
	}
	if c := capability(t, report, 5); c.Value != nil || c.Status == "available" {
		t.Fatalf("old measurement leaked: %+v", c)
	}
}
