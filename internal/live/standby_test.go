package live

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestEmptyCapabilitiesAndATResponsesDoNotConfirmECU(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	for _, e := range []record.Event{
		{Type: "capabilities", Data: probe.RecordingCapabilities{Report: probe.Report{Adapter: "ELM327 v2.3"}}},
		{Type: "command", Data: probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "ATRV", Status: elm327.OK, CompletedAt: time.Now()}, Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(14.0)}}}},
	} {
		if err := s.consume(e); err != nil {
			t.Fatal(err)
		}
		if s.latest.Health == telemetry.Connected || !s.latest.CapturedAt.IsZero() {
			t.Fatalf("adapter response fabricated ECU connection: %+v", s.latest)
		}
	}
}

func TestStandbyPreservesFuelTotalsWithoutBridgingSkippedEvents(t *testing.T) {
	t.Parallel()
	for _, observeStandby := range []bool{false, true} {
		t.Run(fmt.Sprint(observeStandby), func(t *testing.T) {
			t.Parallel()
			s := newSource(nil, "ble")
			now := time.Now()
			s.now = func() time.Time { return now }
			c, err := fuel.Open(fuel.DefaultConfig(), telemetry.Live, filepath.Join(t.TempDir(), "fuel.json"), false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := c.Close(); err != nil {
					t.Error(err)
				}
			})
			consume := func(kind string, data any) {
				t.Helper()
				if err := s.consume(record.Event{Type: kind, Data: data}); err != nil {
					t.Fatal(err)
				}
			}
			measure := func() {
				t.Helper()
				for _, sample := range []struct {
					command string
					value   float64
				}{{"015E", 3.6}, {"010D", 36}} {
					consume("command", probe.RecordingCommand{CommandLog: probe.CommandLog{Command: sample.command, Status: elm327.OK, CompletedAt: now}, Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(sample.value)}}})
				}
				c.Observe(now, s.latest.Clone())
			}
			consume("segment_start", nil)
			consume("capabilities", probe.RecordingCapabilities{})
			measure()
			now = now.Add(time.Second)
			measure()
			before := c.View(now)
			if before.Liters <= 0 || before.DistanceKM <= 0 {
				t.Fatalf("no initial accounting: %+v", before)
			}
			now = now.Add(500 * time.Millisecond)
			consume("standby", probe.Standby{Reason: "ecu"})
			if observeStandby {
				c.Observe(now, s.latest.Clone())
				v := c.View(now)
				if v.FlowLPH != nil || v.InstantL100 != nil || v.Liters != before.Liters || v.DistanceKM != before.DistanceKM {
					t.Fatalf("standby changed totals or retained flow: %+v", v)
				}
			}
			now = now.Add(500 * time.Millisecond)
			consume("segment_start", nil)
			consume("capabilities", probe.RecordingCapabilities{})
			measure()
			v := c.View(now)
			if v.Liters != before.Liters || v.DistanceKM != before.DistanceKM || !v.Incomplete || v.ResetAt != before.ResetAt {
				t.Fatalf("bridged standby or reset totals: %+v", v)
			}
			now = now.Add(time.Second)
			measure()
			if c.View(now).Liters <= before.Liters {
				t.Fatal("accounting did not resume")
			}
		})
	}
}

func TestATActivityCannotRefreshECUFreshness(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	for _, e := range []record.Event{
		{Type: "capabilities", Data: probe.RecordingCapabilities{}},
		{Type: "command", Data: probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "010C", Status: elm327.OK, CompletedAt: now}, Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(0.0)}}}},
	} {
		if err := s.consume(e); err != nil {
			t.Fatal(err)
		}
	}
	if s.latest.Health != telemetry.Connected || s.latest.RPM == nil || *s.latest.RPM != 0 {
		t.Fatal("valid zero RPM is unavailable")
	}
	now = now.Add(4 * time.Second)
	if err := s.consume(record.Event{Type: "command", Data: probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "ATI", CompletedAt: now, Status: elm327.OK}}}); err != nil {
		t.Fatal(err)
	}
	if got := s.latest.At(now, 3*time.Second); got.Health != telemetry.Stale || got.RPM != nil {
		t.Fatalf("AT activity refreshed ECU readings: %+v", got)
	}
}

func TestStandbyClearsMeasurementsAndDistinguishesAdapter(t *testing.T) {
	t.Parallel()
	for _, reason := range []string{"ecu", "adapter", "synchronize"} {
		t.Run(reason, func(t *testing.T) {
			t.Parallel()
			s := newSource(nil, "ble")
			s.latest.RPM = new(900.0)
			s.latest.Signals["rpm"] = telemetry.Signal{Value: new(900.0), Status: "available"}
			s.latest.Health = telemetry.Connected
			if err := s.consume(record.Event{Type: "standby", Data: probe.Standby{Reason: reason}}); err != nil {
				t.Fatal(err)
			}
			if s.latest.Source != telemetry.Live || s.latest.Health != telemetry.Connecting || s.latest.RPM != nil || s.latest.Signals["rpm"].Value != nil {
				t.Fatalf("stale standby state: %+v", s.latest)
			}
			if reason == "ecu" && !strings.Contains(s.latest.Message, "возможно, зажигание выключено") {
				t.Fatal(s.latest.Message)
			}
			if reason != "ecu" && strings.Contains(s.latest.Message, "зажигание") {
				t.Fatal("transport failure misidentified as ignition off")
			}
		})
	}
}
