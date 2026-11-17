package mil

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

type memoryWriter struct {
	entries []milstore.Entry
	err     error
	closes  int
}

func (w *memoryWriter) Submit(e milstore.Entry) error {
	if w.err != nil {
		return w.err
	}
	e.Data = slices.Clone(e.Data)
	w.entries = append(w.entries, e)
	return nil
}
func (w *memoryWriter) Status() milstore.Status { return milstore.Status{State: "recording"} }
func (w *memoryWriter) Close() error            { w.closes++; return w.err }

func captureKind(w *memoryWriter, kind string) []milstore.Entry {
	var result []milstore.Entry
	for _, e := range w.entries {
		if e.Kind == kind {
			result = append(result, e)
		}
	}
	return result
}

func TestCapturePrePostAndEventsWithoutBrowser(t *testing.T) {
	t.Parallel()
	at := time.Now()
	w := &memoryWriter{}
	c := NewCapture(w, nil, 3*time.Second)
	for i := range 71 {
		now := at.Add(time.Duration(i) * time.Second)
		c.ObserveAt(now, synthetic(now, false))
	}
	onAt := at.Add(70250 * time.Millisecond)
	c.ObserveAt(onAt, synthetic(onAt, true))
	for i := 71; i <= 100; i++ {
		now := at.Add(time.Duration(i) * time.Second)
		c.ObserveAt(now, synthetic(now, true))
	}
	c.Tick(at.Add(101 * time.Second))
	start, end := captureKind(w, "incident_start"), captureKind(w, "incident_end")
	if len(start) != 1 || len(end) != 1 {
		t.Fatal(len(start), len(end))
	}
	var window Window
	if err := json.Unmarshal(end[0].Data, &window); err != nil {
		t.Fatal(err)
	}
	if !window.PreWindowRetained || !window.PostWindowRetained || window.Reason != "window_elapsed" {
		t.Fatalf("incomplete window: %+v", window)
	}
	pre, post := captureKind(w, "pre"), captureKind(w, "post")
	if len(pre) < 60 || len(post) < 29 {
		t.Fatal("lost context", len(pre), len(post))
	}
	// OFF and loss after capture ends are still persisted as independent events.
	now := at.Add(110 * time.Second)
	c.ObserveAt(now, synthetic(now, false))
	c.Tick(now.Add(4 * time.Second))
	if len(captureKind(w, "event")) != 4 {
		t.Fatal("lost OFF/UNKNOWN outside active capture", captureKind(w, "event"))
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil || w.closes != 1 {
		t.Fatal(err, w.closes)
	}
}

func TestCaptureAlreadyOnPartialCloseAndOverlap(t *testing.T) {
	t.Parallel()
	at := time.Now()
	w := &memoryWriter{}
	c := NewCapture(w, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, true))
	c.ObserveAt(at.Add(5*time.Second), synthetic(at.Add(5*time.Second), false))
	c.ObserveAt(at.Add(10*time.Second), synthetic(at.Add(10*time.Second), true))
	if len(captureKind(w, "incident_start")) != 1 || len(captureKind(w, "incident_extend")) != 1 {
		t.Fatal("overlap not bounded/shared", w.entries)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	var window Window
	ends := captureKind(w, "incident_end")
	if len(ends) != 1 {
		t.Fatal(ends)
	}
	if err := json.Unmarshal(ends[0].Data, &window); err != nil {
		t.Fatal(err)
	}
	if window.PreWindowRetained || window.PostWindowRetained || window.Reason != "shutdown" {
		t.Fatal("partial context reported complete", window)
	}
	var event Event
	if err := json.Unmarshal(captureKind(w, "event")[0].Data, &event); err != nil {
		t.Fatal(err)
	}
	if event.Kind != "MIL_ALREADY_ON" || !event.PreviousCapturedAt.IsZero() {
		t.Fatal(event)
	}
}

func TestCaptureOutputFailureDoesNotStopMonitor(t *testing.T) {
	t.Parallel()
	at := time.Now()
	cause := errors.New("synthetic disk failure")
	w := &memoryWriter{err: cause}
	c := NewCapture(w, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, false))
	c.ObserveAt(at.Add(time.Second), synthetic(at.Add(time.Second), true))
	status := c.Status()
	if status.State != "failed" || status.LastEvent == nil || status.LastEvent.Kind != "MIL_ON" {
		t.Fatal(status)
	}
	if err := c.Close(); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}

func TestDelayedTickCannotFabricatePostWindow(t *testing.T) {
	t.Parallel()
	at := time.Now()
	w := &memoryWriter{}
	c := NewCapture(w, nil, 3*time.Second)
	c.ObserveAt(at, synthetic(at, true))
	c.Tick(at.Add(100 * time.Second))
	var window Window
	if err := json.Unmarshal(captureKind(w, "incident_end")[0].Data, &window); err != nil {
		t.Fatal(err)
	}
	if window.PostWindowRetained || !window.HadGaps {
		t.Fatal("fabricated 30s post history after timer gap", window)
	}
	var frame Frame
	posts := captureKind(w, "post")
	if err := json.Unmarshal(posts[len(posts)-1].Data, &frame); err != nil {
		t.Fatal(err)
	}
	if !frame.At.Equal(at.Add(100 * time.Second)) {
		t.Fatal("invented observation at old deadline", frame.At)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
