package probe

import (
	"context"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func TestReportedModelIsSeparateFromAdapterIdentity(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, model, source string }{
		{"owner correction", "Vgate iCar Pro BLE 4.0 Dual", "owner_reported"},
		{"another adapter", "another ELM adapter", "user_supplied"},
		{"unknown model", "", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := fixtureAdapter(t)
			runner, err := New(Config{AdapterModel: tc.model}, func(context.Context) (elm327.Transport, error) { return a, nil })
			if err != nil {
				t.Fatal(err)
			}
			report, err := runner.Run(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if report.Device.Model != tc.model || report.Device.ModelSource != tc.source || report.Transport != "serial_spp" {
				t.Fatalf("incorrect device context: %+v transport=%s", report.Device, report.Transport)
			}
			if report.Adapter != "ELM327 v2.3" {
				t.Fatal("owner model replaced the actual ATI response")
			}
			if tc.source == "owner_reported" && report.Device.EvidenceURL != "https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184" {
				t.Fatal("owner report lacks provenance")
			}
			if tc.source != "owner_reported" && report.Device.EvidenceURL != "" {
				t.Fatal("model was assigned unrelated evidence")
			}
		})
	}
}
