package live

import (
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestObserverSeesEveryMILCommandAndLossWithoutReader(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	at := time.Now()
	s.now = func() time.Time { return at }
	var seen []telemetry.Snapshot
	if err := s.SetObserver(func(snapshot telemetry.Snapshot) { seen = append(seen, snapshot) }); err != nil {
		t.Fatal(err)
	}
	if err := s.consume(record.Event{Type: "capabilities", Data: probe.RecordingCapabilities{}}); err != nil {
		t.Fatal(err)
	}
	for _, on := range []bool{false, true, false} {
		at = at.Add(time.Second)
		data := probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "0101", CompletedAt: at, Status: elm327.OK, RawResponse: "synthetic"}, LatencyNS: 1234,
			Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{MIL: new(on), DTCCount: new(0)}}}
		if err := s.consume(record.Event{Type: "command", Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 4 || !*seen[2].MIL.On || *seen[3].MIL.On || seen[3].Poll.LatencyNS != 1234 {
		t.Fatal("coalesced MIL observations", seen)
	}
	*seen[3].MIL.On = true
	if *s.latest.MIL.On {
		t.Fatal("observer aliases source")
	}
	if err := s.consume(record.Event{Type: "gap"}); err != nil {
		t.Fatal(err)
	}
	if seen[4].MIL.On != nil || seen[4].MIL.Status == "available" {
		t.Fatal("lost connection retained live MIL")
	}
	if err := s.SetObserver(func(telemetry.Snapshot) {}); err == nil {
		t.Fatal("observer replaced")
	}
}

func TestStoredContextCannotChangeLiveSignals(t *testing.T) {
	t.Parallel()
	s := newSource(nil, "ble")
	at := time.Now()
	data := probe.RecordingCommand{CommandLog: probe.CommandLog{Command: "020C00", CompletedAt: at, Status: elm327.OK, RawResponse: "420C001000\r>"},
		Result: &probe.RecordingReading{Status: "available", Measurement: elm327.Measurement{Value: new(1024.0), Unit: "rpm"}}}
	if err := s.consume(record.Event{Type: "command", Data: data}); err != nil {
		t.Fatal(err)
	}
	if s.latest.RPM != nil || s.latest.ECUReadings["020C00"].Status != "available" || s.latest.Poll.Command != "020C00" {
		t.Fatal("stored frame became live RPM", s.latest)
	}
}
