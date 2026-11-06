package mil

import (
	"encoding/json"
	"testing"
	"time"
)

// Synthetic ordering reproduces the recorded negative-age case: a timer time
// is sampled, a newer source publication is consumed, then the timer runs.
func TestCaptureOldTickCannotInvalidateNewPublication(t *testing.T) {
	t.Parallel()
	for _, newMIL := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_core_only", true: "new_mil"}[newMIL], func(t *testing.T) {
			t.Parallel()
			at := time.Now()
			w := &memoryWriter{}
			c := NewCapture(w, nil, 3*time.Second)
			c.ObserveAt(at, synthetic(at, false))
			tickAt := at.Add(time.Second)
			publicationAt := tickAt.Add(2 * time.Millisecond)
			publication := synthetic(publicationAt, false)
			if !newMIL {
				publication.MIL = synthetic(at, false).MIL
			}
			c.ObserveAt(publicationAt, publication)
			c.Tick(tickAt)
			c.Tick(publicationAt.Add(250 * time.Millisecond))
			events := captureKind(w, "event")
			if len(events) != 1 || c.Status().LastEvent.Kind != "MIL_INITIAL_OFF" {
				t.Fatalf("old timer fabricated loss: events=%d last=%+v", len(events), c.Status().LastEvent)
			}
			// Genuine silence must still expire independently of the ignored old tick.
			c.Tick(publicationAt.Add(3 * time.Second))
			if e := c.Status().LastEvent; e.Kind != "MIL_UNKNOWN" || e.Observation.On != nil {
				t.Fatalf("real silence no longer expires: %+v", e)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCaptureOldTickPreservesConfirmedMILOn(t *testing.T) {
	t.Parallel()
	at := time.Now()
	w := &memoryWriter{}
	c := NewCapture(w, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, false))
	latest := at.Add(time.Second)
	c.ObserveAt(latest, synthetic(latest, false))
	c.Tick(latest.Add(-time.Millisecond))
	onAt := at.Add(2 * time.Second)
	c.ObserveAt(onAt, synthetic(onAt, true))
	e := c.Status().LastEvent
	if e.Kind != "MIL_ON" || !e.PreviousCapturedAt.Equal(latest) || e.ID != 2 {
		t.Fatalf("old tick broke confirmed transition: %+v", e)
	}
	var window Window
	if err := json.Unmarshal(captureKind(w, "incident_start")[0].Data, &window); err != nil {
		t.Fatal(err)
	}
	if window.HadGaps {
		t.Fatal("old timer fabricated a pre-context gap")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureStillRejectsFutureEvidenceAndExpiresMIL(t *testing.T) {
	t.Parallel()
	at := time.Now()
	c := NewCapture(&memoryWriter{}, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, false))
	// This is genuinely future input relative to the observation, rather than
	// an older timer processing a snapshot that was already published.
	c.ObserveAt(at.Add(time.Second), synthetic(at.Add(time.Hour), false))
	if e := c.Status().LastEvent; e.Kind != "MIL_UNKNOWN" || e.Observation.On != nil {
		t.Fatalf("future evidence became live: %+v", e)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c = NewCapture(&memoryWriter{}, nil, 3*time.Second)
	s := synthetic(at, false)
	c.ObserveAt(at, s)
	now := at.Add(15 * time.Second)
	s.CapturedAt, s.ReceivedAt = now, now // Core is fresh, MIL itself has expired.
	c.ObserveAt(now, s)
	if e := c.Status().LastEvent; e.Kind != "MIL_UNKNOWN" || e.Observation.On != nil {
		t.Fatalf("expired MIL became live with fresh core: %+v", e)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureNoDataStillEndsKnownMIL(t *testing.T) {
	t.Parallel()
	at := time.Now()
	c := NewCapture(&memoryWriter{}, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, false))
	s := synthetic(at.Add(time.Second), false)
	s.MIL.Status, s.MIL.On, s.MIL.DTCCount = "temporarily_unavailable", nil, nil
	s.MIL.RawResponse = "NO DATA\r>"
	c.ObserveAt(at.Add(time.Second), s)
	if e := c.Status().LastEvent; e.Kind != "MIL_UNKNOWN" || e.Observation.Status != "temporarily_unavailable" || e.Trigger() {
		t.Fatalf("no data did not end known MIL: %+v", e)
	}
	s = synthetic(at.Add(2*time.Second), false)
	c.ObserveAt(at.Add(2*time.Second), s)
	if e := c.Status().LastEvent; e.Kind != "MIL_OFF_AFTER_GAP" {
		t.Fatal(e)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
