package live

import (
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func TestExistingPIDRepliesPopulateDiagnosticSignals(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	for _, sample := range []struct {
		command, key string
		value        float64
	}{{"0111", "throttle_percent", 20}, {"0106", "stft_b1_percent", 3}, {"0107", "ltft_b1_percent", -1}, {"0114", "o2_sensor1_v", .6}} {
		data := probe.RecordingCommand{CommandLog: probe.CommandLog{Command: sample.command, Status: elm327.OK, CompletedAt: time.Now()}, Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(sample.value), Extra: map[string]float64{"short_fuel_trim_percent": 5}}}}
		if err := s.consume(record.Event{Type: "command", Data: data}); err != nil {
			t.Fatal(err)
		}
		signal := s.latest.Signals[sample.key]
		if signal.Value == nil || *signal.Value != sample.value || signal.Source != "ecu_mode01" {
			t.Fatalf("lost existing reply %s: %+v", sample.command, signal)
		}
	}
	if v := s.latest.Signals["o2_sensor1_stft_percent"].Value; v == nil || *v != 5 {
		t.Fatal("lost O2 extra")
	}
}
