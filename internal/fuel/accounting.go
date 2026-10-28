package fuel

import (
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type endpoint struct {
	at                    time.Time
	flow, speed           *float64
	flowUntil, speedUntil time.Time
	generation            uint64
	method                string
}

func (c *Computer) endpoint(now time.Time) endpoint {
	s := c.latest.At(now, c.cfg.StaleAfter)
	c.result = Evaluate(c.cfg, s, c.method, now)
	c.speed, c.speedUntil = nil, time.Time{}
	if s.Source != c.state.Source {
		c.result = unavailable("source_mismatch")
		return endpoint{at: now}
	}
	if c.method != "" && c.state.Method == "" {
		c.state.Method = c.method
		c.dirty = true
	}
	if c.method != "" && c.method != c.state.Method {
		c.result = unavailable("model_changed_reset_required")
	}
	if s.Health == telemetry.Connected {
		c.speed, c.speedUntil = signal(s, "speed_kmh", 0, 220)
		if c.speed != nil {
			c.speedUntil = horizon(c.cfg, s, c.speedUntil)
		}
	}
	return endpoint{at: now, flow: c.result.Flow, speed: c.speed, flowUntil: c.result.Until, speedUntil: c.speedUntil, generation: s.Generation, method: c.method}
}

func (c *Computer) advance(now time.Time) {
	next := c.endpoint(now)
	if c.previous == nil {
		if next.flow != nil || next.speed != nil {
			c.previous = &next
		}
		return
	}
	prev := *c.previous
	dt := now.Sub(prev.at)
	if dt == 0 {
		c.previous = &next
		return
	}
	if dt < 0 {
		c.state.mark("clock_reversal")
		c.previous = &next
		c.dirty = true
		return
	}
	same := dt <= 3*time.Second && next.generation == prev.generation && next.method == prev.method
	fuelOK := same && prev.flow != nil && next.flow != nil && now.Before(prev.flowUntil)
	speedOK := same && prev.speed != nil && next.speed != nil && now.Before(prev.speedUntil)
	liters, km := 0.0, 0.0
	hours := dt.Hours()
	if fuelOK {
		liters = (*prev.flow + *next.flow) / 2 * hours
		c.state.BaseLiters += liters
	}
	if speedOK {
		km = (*prev.speed + *next.speed) / 2 * hours
		c.state.DistanceKM += km
	}
	c.state.ObservedNS += dt.Nanoseconds()
	if fuelOK && speedOK {
		c.state.JointLiters += liters
		c.state.CoveredKM += km
		c.state.CoveredNS += dt.Nanoseconds()
	}
	if !fuelOK || !speedOK {
		c.state.GapNS += dt.Nanoseconds()
		reason := "unavailable_input"
		if !same {
			reason = "timing_or_generation_gap"
		}
		c.state.mark(reason)
	}
	c.previous = &next
	c.dirty = true
	// Save failures remain visible in View; Close and local mutations return them.
	_ = c.save(now, false)
}
