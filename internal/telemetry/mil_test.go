package telemetry

import (
	"testing"
	"time"
)

func TestMILExpiresIndependentlyAndCloneOwnsContext(t *testing.T) {
	t.Parallel()
	now := time.Now()
	s := Snapshot{Health: Connected, CapturedAt: now, ReceivedAt: now,
		MIL:         &MILReading{On: new(true), DTCCount: new(0), Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 5000, Readiness: []byte(`{"ignition":"spark"}`)},
		ECUReadings: map[string]ECUReading{"03": {Status: "available", Detail: []byte(`{"codes":[]}`)}},
	}
	v := s.At(now.Add(6*time.Second), time.Minute)
	if v.Health != Connected || v.MIL.On != nil || v.MIL.Status != "stale" || *v.MIL.AgeMS != 6000 {
		t.Fatalf("stale MIL presented live: %+v", v)
	}
	c := s.Clone()
	*c.MIL.On = false
	c.MIL.Readiness[0] = 'x'
	r := c.ECUReadings["03"]
	r.Detail[0] = 'x'
	if !*s.MIL.On || s.MIL.Readiness[0] != '{' || s.ECUReadings["03"].Detail[0] != '{' {
		t.Fatal("clone aliases mutable diagnostic context")
	}
	s.Health = Disconnected
	if s.At(now, time.Minute).MIL.On != nil {
		t.Fatal("disconnected MIL remained available")
	}
}
