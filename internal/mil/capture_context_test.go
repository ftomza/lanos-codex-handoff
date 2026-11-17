package mil

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestCaptureReadsFuelWithoutAdvancingIt(t *testing.T) {
	t.Parallel()
	at := time.Now()
	computer, err := fuel.Open(fuel.DefaultConfig(), telemetry.Simulator, filepath.Join(t.TempDir(), "fuel.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer computer.Close()
	for i := range 2 {
		now := at.Add(time.Duration(i) * time.Second)
		s := synthetic(now, false)
		s.Source = telemetry.Simulator
		s.Signals = map[string]telemetry.Signal{
			"fuel_rate_lph": {Value: new(3.6), Unit: "L/h", Source: "ecu_mode01", Support: "supported", Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
			"speed_kmh":     {Value: new(36.0), Unit: "km/h", Source: "ecu_mode01", Support: "supported", Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
		}
		computer.Observe(now, s)
	}
	before := computer.Diagnostic(at.Add(time.Second))
	if before.Fuel.Liters <= 0 {
		t.Fatal("fixture did not integrate fuel")
	}
	w := &memoryWriter{}
	c := NewCapture(w, func(now time.Time) (json.RawMessage, error) { return json.Marshal(computer.Diagnostic(now)) }, 3*time.Second)
	for i := 10; i < 20; i++ {
		now := at.Add(time.Duration(i) * time.Second)
		s := synthetic(now, true)
		s.Source = telemetry.Simulator
		c.ObserveAt(now, s)
	}
	if after := computer.Diagnostic(at.Add(time.Second)); !reflect.DeepEqual(before, after) {
		t.Fatal("recording mutated fuel state")
	}
	var frame Frame
	if err := json.Unmarshal(captureKind(w, "pre")[0].Data, &frame); err != nil {
		t.Fatal(err)
	}
	var context fuel.Diagnostic
	if err := json.Unmarshal(frame.FuelContext, &context); err != nil {
		t.Fatal(err)
	}
	if !context.Snapshot.CapturedAt.Equal(at.Add(time.Second)) || context.At.Equal(context.Snapshot.CapturedAt) || context.Fuel.Liters != before.Fuel.Liters {
		t.Fatal("invented coherent fuel input time", context)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureHardDeadlineAndLateDiagnostic(t *testing.T) {
	t.Parallel()
	at := time.Now()
	w := &memoryWriter{}
	c := NewCapture(w, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, true))
	for i := 1; i < 120; i++ {
		now := at.Add(time.Duration(i) * time.Second)
		on := i%20 != 0
		c.ObserveAt(now, synthetic(now, on))
	}
	c.Tick(at.Add(120 * time.Second))
	if c.Status().ActiveIncident != "" || len(captureKind(w, "incident_start")) != 1 {
		t.Fatal("unbounded overlap", c.Status())
	}
	now := at.Add(125 * time.Second)
	s := synthetic(now, true)
	s.Poll = &telemetry.Poll{Command: "0A", At: now, Status: "unsupported"}
	s.ECUReadings = map[string]telemetry.ECUReading{"0A": {Status: "unsupported", CapturedAt: now, ReceivedAt: now, RawResponse: "7F0A11\r>"}}
	c.ObserveAt(now, s)
	diagnostics := captureKind(w, "diagnostic")
	if len(diagnostics) != 1 || diagnostics[0].Incident != "" {
		t.Fatal("late diagnostic lost or associated by guess", diagnostics)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
