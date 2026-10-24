package probe

import (
	"context"
	"slices"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func TestAdapterFailureAndECUUnavailableAreDifferent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, command, raw, status string
		fatal                      bool
	}{
		{"adapter rejected echo setting", "ATE0", "?\r>", "adapter_error", true},
		{"ECU not connected", "0100", "UNABLE TO CONNECT\r>", "ecu_unavailable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			a.responses[tc.command] = tc.raw
			r, err := New(Config{}, func(context.Context) (elm327.Transport, error) { return a, nil })
			if err != nil {
				t.Fatal(err)
			}
			report, err := r.Run(t.Context())
			if (err != nil) != tc.fatal {
				t.Fatalf("fatal=%t, got %v", tc.fatal, err)
			}
			found := false
			for _, c := range report.Commands {
				if c.Command == tc.command && string(c.Status) == tc.status {
					found = true
				}
			}
			if !found || slices.Contains(a.commands, "010C") {
				t.Fatalf("wrong error layer or unsafe polling: %v", a.commands)
			}
			if c := capability(t, report, 0x0C); c.Value != nil || c.Support != "not_yet_tested" {
				t.Fatalf("failed discovery fabricated support: %+v", c)
			}
		})
	}
}
