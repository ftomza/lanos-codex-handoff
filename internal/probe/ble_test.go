package probe

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func TestBLEUsesSharedScanWithoutReset(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.responses["0120"] = "41 20 C0 00 00 00 \r\r>"
	a.responses["010C"] = "41 0C 0D 43 \r\r>"
	a.responses["ATRV"] = "10.9V\r\r>"
	var deadline time.Duration
	r, err := New(Config{Transport: "ble", BLEName: "IOS-Vlink", ScanSupported: true,
		Timeout: time.Second, ConnectTimeout: 25 * time.Second}, func(ctx context.Context) (elm327.Transport, error) {
		end, _ := ctx.Deadline()
		deadline = time.Until(end)
		return a, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if deadline < 24*time.Second || report.Transport != "ble_gatt" || report.Source != "elm327_ble" || report.BLE == nil || report.BLE.Name != "IOS-Vlink" {
		t.Fatalf("BLE context: deadline=%s report=%+v", deadline, report)
	}
	if report.Port != "" || report.Baud != 0 || report.Adapter != "ELM327 v2.3" || report.AdapterVoltage.Value == nil || *report.AdapterVoltage.Value != 10.9 {
		t.Fatalf("metadata/readings: %+v", report)
	}
	if c := capability(t, report, 0x0C); c.Value == nil || *c.Value != 848.75 {
		t.Fatalf("RPM: %+v", c)
	}
	for _, blocked := range []string{"ATZ", "0140", "0142", "012F", "04"} {
		if slices.Contains(a.commands, blocked) {
			t.Fatalf("unexpected command %s", blocked)
		}
	}
	if !slices.Equal(a.commands[:4], []string{"ATE0", "ATL0", "ATI", "ATSP0"}) || !a.closed {
		t.Fatalf("initialization/closure: %v", a.commands)
	}
}

func TestUnknownTransportRejected(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Transport: "guess"}, func(context.Context) (elm327.Transport, error) { return nil, nil }); err == nil {
		t.Fatal("unknown transport accepted")
	}
}
