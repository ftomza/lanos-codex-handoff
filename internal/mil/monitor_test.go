package mil

import (
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func synthetic(at time.Time, on bool) telemetry.Snapshot {
	return telemetry.Snapshot{Source: telemetry.Live, Generation: 1, Health: telemetry.Connected, CapturedAt: at, ReceivedAt: at,
		MIL: &telemetry.MILReading{On: new(on), DTCCount: new(0), Status: "available", RawResponse: "synthetic", CapturedAt: at, ReceivedAt: at, StaleAfterMS: 15000}}
}

func TestMonitorTransitionsLossResumeAndDedup(t *testing.T) {
	t.Parallel()
	at := time.Now()
	m := NewMonitor(3 * time.Second)
	s := synthetic(at, false)
	if events := m.Observe(at, s); len(events) != 1 || events[0].Kind != "MIL_INITIAL_OFF" {
		t.Fatal(events)
	}
	s = synthetic(at.Add(5*time.Second), true)
	events := m.Observe(at.Add(5*time.Second), s)
	if len(events) != 1 || events[0].Kind != "MIL_ON" || !events[0].Trigger() || !events[0].PreviousCapturedAt.Equal(at) {
		t.Fatal(events)
	}
	if events := m.Observe(at.Add(6*time.Second), s); len(events) != 0 {
		t.Fatal("duplicate capture", events)
	}
	if events := m.Tick(at.Add(21 * time.Second)); len(events) != 1 || events[0].Kind != "MIL_UNKNOWN" {
		t.Fatal("expired without loss event", events)
	}
	s = synthetic(at.Add(22*time.Second), true)
	events = m.Observe(at.Add(22*time.Second), s)
	if len(events) != 1 || events[0].Kind != "MIL_ON_AFTER_GAP" {
		t.Fatal("invented transition through gap", events)
	}
	s = synthetic(at.Add(27*time.Second), false)
	if events := m.Observe(at.Add(27*time.Second), s); len(events) != 1 || events[0].Kind != "MIL_OFF" {
		t.Fatal(events)
	}
	s.Generation++
	s.MIL.CapturedAt, s.MIL.ReceivedAt = at.Add(28*time.Second), at.Add(28*time.Second)
	if events := m.Observe(at.Add(28*time.Second), s); len(events) != 2 || events[1].Kind != "MIL_OFF_AFTER_GAP" {
		t.Fatal("generation continuity invented", events)
	}
}

func TestMonitorAlreadyOnAndDisconnected(t *testing.T) {
	t.Parallel()
	at := time.Now()
	m := NewMonitor(3 * time.Second)
	s := synthetic(at, true)
	e := m.Observe(at, s)
	if len(e) != 1 || e[0].Kind != "MIL_ALREADY_ON" || !e[0].PreviousCapturedAt.IsZero() {
		t.Fatal(e)
	}
	s.Health = telemetry.Disconnected
	if e := m.Observe(at.Add(time.Second), s); len(e) != 1 || e[0].State != "UNKNOWN" || e[0].Trigger() {
		t.Fatal(e)
	}
	if e := m.Observe(at.Add(2*time.Second), s); len(e) != 0 {
		t.Fatal("duplicate loss", e)
	}
	s.Generation++
	if e := m.Observe(at.Add(3*time.Second), s); len(e) != 0 {
		t.Fatal("reconnect duplicated loss event", e)
	}
}

func TestRingRetainsBoundaryPredecessorForFractionalEvent(t *testing.T) {
	t.Parallel()
	at := time.Now()
	r := NewRing()
	for i := range 71 {
		r.Add(Frame{At: at.Add(time.Duration(i) * time.Second)}, false)
	}
	eventAt := at.Add(70250 * time.Millisecond)
	r.Add(Frame{At: eventAt}, true)
	frames, complete := r.Before(eventAt)
	if !complete || !frames[0].At.Equal(at.Add(10*time.Second)) {
		t.Fatalf("fractional event lost covered pre-window: complete=%t frames=%d first=%s", complete, len(frames), frames[0].At)
	}
}

func TestMonitorDoesNotReusePreGapOrDuplicateMIL(t *testing.T) {
	t.Parallel()
	at := time.Now()
	m := NewMonitor(3 * time.Second)
	s := synthetic(at, false)
	m.Observe(at, s)
	*s.MIL.On = true // Conflicting value with an already-consumed capture ID.
	if e := m.Observe(at.Add(time.Second), s); len(e) != 0 {
		t.Fatal("duplicate capture created transition", e)
	}
	m.Tick(at.Add(4 * time.Second))
	s.CapturedAt, s.ReceivedAt = at.Add(5*time.Second), at.Add(5*time.Second)
	if e := m.Observe(at.Add(5*time.Second), s); len(e) != 0 {
		t.Fatal("core recovery reused MIL from before gap", e)
	}
	s = synthetic(at.Add(6*time.Second), true)
	if e := m.Observe(at.Add(6*time.Second), s); len(e) != 1 || e[0].Kind != "MIL_ON_AFTER_GAP" {
		t.Fatal(e)
	}
}

func TestRingWindowBoundsAndOwnership(t *testing.T) {
	t.Parallel()
	at := time.Now()
	r := NewRing()
	for i := range 70 {
		s := synthetic(at.Add(time.Duration(i)*time.Second), false)
		r.Add(Frame{At: s.CapturedAt, Snapshot: s}, false)
	}
	frames, complete := r.Before(at.Add(69 * time.Second))
	if !complete || len(frames) != 61 || !frames[0].At.Equal(at.Add(9*time.Second)) {
		t.Fatal(len(frames), complete)
	}
	*frames[0].Snapshot.MIL.On = true
	again, _ := r.Before(at.Add(69 * time.Second))
	if *again[0].Snapshot.MIL.On {
		t.Fatal("ring aliases returned context")
	}
	for i := range 1000 {
		r.Add(Frame{At: at.Add(70*time.Second + time.Duration(i)*time.Millisecond)}, true)
	}
	frames, complete = r.Before(at.Add(71 * time.Second))
	if complete || len(frames) > 256 {
		t.Fatal("unbounded or unmarked lost pre-context", len(frames), complete)
	}
}
