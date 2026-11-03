package mil

import (
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func (c *Capture) frame(now time.Time, s telemetry.Snapshot) Frame {
	f := NewFrame(now, s, c.staleAfter)
	if c.fuel != nil {
		data, err := c.fuel(now)
		if err != nil && c.err == nil {
			c.err = err
		}
		f.FuelContext = data
	}
	return f
}

func (c *Capture) process(now time.Time, s telemetry.Snapshot, events []Event) {
	newPoll := s.Poll != nil && *s.Poll != c.lastPoll
	diagnostic := newPoll && (s.Poll.Command == "03" || s.Poll.Command == "07" || s.Poll.Command == "0A" || len(s.Poll.Command) == 6 && s.Poll.Command[:2] == "02")
	force := len(events) > 0 || diagnostic || s.Diagnostics.Errors > c.lastErrors || s.Health != c.lastHealth || s.Generation != c.lastGen
	f := c.frame(now, s)
	added := c.ring.Add(f, force)
	opened := false
	for _, event := range events {
		if event.Trigger() {
			if c.active == nil {
				c.begin(now, event)
				opened = true
			} else {
				c.active.PostUntil = minTime(c.active.TriggeredAt.Add(120*time.Second), now.Add(PostWindow))
				c.submit("incident_extend", now, c.active.ID, c.active)
			}
		}
		c.lastEvent = &event
		c.submit("event", now, c.incidentID(), event)
	}
	if added && c.active != nil && !opened {
		c.recordFrame("post", f)
	}
	// Jobs may finish after the post window. Empty incident IDs avoid guessing
	// association, while these independent rows preserve late diagnostic results.
	if diagnostic {
		if r, ok := s.ECUReadings[s.Poll.Command]; ok {
			c.submit("diagnostic", now, c.incidentID(), struct {
				Command string               `json:"command"`
				Reading telemetry.ECUReading `json:"reading"`
				Poll    telemetry.Poll       `json:"poll"`
			}{s.Poll.Command, r, *s.Poll})
		}
	}
	if s.Poll != nil {
		c.lastPoll = *s.Poll
	}
	c.lastErrors, c.lastHealth, c.lastGen = s.Diagnostics.Errors, s.Health, s.Generation
}

func (c *Capture) begin(now time.Time, event Event) {
	frames, retained := c.ring.Before(now)
	c.active = &Window{ID: incidentName(event.ID), TriggeredAt: now.UTC(), RequestedPreFrom: now.Add(-PreWindow).UTC(), PostUntil: now.Add(PostWindow).UTC(), PreWindowRetained: retained}
	c.lastPostAt, c.postGap = now, false
	if len(frames) > 0 {
		c.active.RetainedFrom = frames[0].At
	}
	for i, f := range frames {
		c.active.HadGaps = c.active.HadGaps || f.EffectiveHealth != telemetry.Connected
		if i > 0 && f.At.Sub(frames[i-1].At) > 2*time.Second {
			c.active.HadGaps = true
		}
	}
	c.submit("incident_start", now, c.active.ID, c.active)
	for _, f := range frames {
		c.recordFrame("pre", f)
	}
}

func (c *Capture) recordFrame(kind string, f Frame) {
	c.active.HadGaps = c.active.HadGaps || f.EffectiveHealth != telemetry.Connected
	if kind == "post" {
		if f.At.Sub(c.lastPostAt) > 2*time.Second {
			c.postGap, c.active.HadGaps = true, true
		}
		c.lastPostAt = f.At
	}
	c.submit(kind, f.At, c.active.ID, f)
}

func (c *Capture) finishDue(now time.Time) {
	if c.active == nil || now.Before(c.active.PostUntil) {
		return
	}
	// Keep actual observation time, including timer delays. A delayed tick cannot
	// fabricate historical observations or a complete missing post window.
	c.recordFrame("post", c.frame(now, c.latest))
	c.active.PostWindowRetained = !c.postGap
	c.end(now, "window_elapsed")
}

func (c *Capture) end(now time.Time, reason string) {
	c.active.EndedAt, c.active.Reason = now.UTC(), reason
	c.submit("incident_end", now, c.active.ID, c.active)
	c.active = nil
}

func (c *Capture) incidentID() string {
	if c.active == nil {
		return ""
	}
	return c.active.ID
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
