package mil

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type eventWriter interface {
	Submit(milstore.Entry) error
	Status() milstore.Status
	Close() error
}

type CaptureStatus struct {
	State          string          `json:"state"`
	Error          string          `json:"error,omitempty"`
	ActiveIncident string          `json:"active_incident,omitempty"`
	LastEvent      *Event          `json:"last_event,omitempty"`
	Store          milstore.Status `json:"store"`
}

// Window flags describe retained time coverage. HadGaps independently reports
// missing/unavailable source data; a covered window does not imply live inputs.
type Window struct {
	ID                 string    `json:"id"`
	TriggeredAt        time.Time `json:"triggered_at"`
	RequestedPreFrom   time.Time `json:"requested_pre_from"`
	RetainedFrom       time.Time `json:"retained_from"`
	PostUntil          time.Time `json:"post_until"`
	EndedAt            time.Time `json:"ended_at,omitzero"`
	PreWindowRetained  bool      `json:"pre_window_retained"`
	PostWindowRetained bool      `json:"post_window_retained"`
	HadGaps            bool      `json:"had_gaps"`
	Reason             string    `json:"reason,omitempty"`
}

// Capture owns its asynchronous writer. Fuel supplies a read-only view with
// its own input times. Observations and ticks never perform file I/O.
type Capture struct {
	mu         sync.Mutex
	monitor    *MILMonitor
	ring       *Ring
	writer     eventWriter
	fuel       func(time.Time) (json.RawMessage, error)
	staleAfter time.Duration
	latest     telemetry.Snapshot
	active     *Window
	lastPostAt time.Time
	postGap    bool
	lastEvent  *Event
	lastPoll   telemetry.Poll
	lastErrors uint64
	lastHealth telemetry.Health
	lastGen    uint64
	err        error
	closed     bool
	now        func() time.Time
	closeOnce  sync.Once
	closeErr   error
}

func NewCapture(writer eventWriter, fuel func(time.Time) (json.RawMessage, error), staleAfter time.Duration) *Capture {
	return &Capture{monitor: NewMonitor(staleAfter), ring: NewRing(), writer: writer, fuel: fuel, staleAfter: staleAfter, now: time.Now}
}

func incidentName(id uint64) string { return fmt.Sprintf("incident-%016x", id) }

func (c *Capture) Observe(snapshot telemetry.Snapshot) { c.ObserveAt(c.now(), snapshot) }

func (c *Capture) ObserveAt(now time.Time, snapshot telemetry.Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.finishDue(now)
	c.latest = snapshot.Clone()
	c.process(now, snapshot, c.monitor.Observe(now, snapshot))
}

func (c *Capture) Tick(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.latest.Source == "" {
		return
	}
	c.finishDue(now)
	c.process(now, c.latest, c.monitor.Tick(now))
}

func (c *Capture) submit(kind string, at time.Time, incident string, value any) {
	if c.err != nil {
		return
	}
	data, err := json.Marshal(value)
	if err == nil {
		err = c.writer.Submit(milstore.Entry{Kind: kind, At: at.UTC(), Incident: incident, Data: data})
	}
	if err != nil {
		c.err = fmt.Errorf("mil recording stopped: %w", err)
	}
}

func (c *Capture) Status() CaptureStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := CaptureStatus{State: "recording", Store: c.writer.Status()}
	if c.closed {
		status.State = "closed"
	}
	if c.active != nil {
		status.ActiveIncident = c.active.ID
	}
	if c.lastEvent != nil {
		e := *c.lastEvent
		e.Observation = (telemetry.Snapshot{MIL: e.Observation}).Clone().MIL
		status.LastEvent = &e
	}
	if c.err != nil {
		status.State, status.Error = "failed", c.err.Error()
	}
	if status.Store.State == "failed" {
		status.State, status.Error = "failed", status.Store.Error
	}
	return status
}

func (c *Capture) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		now := c.now()
		c.finishDue(now)
		c.closed = true
		if c.active != nil {
			c.end(now, "shutdown")
		}
		c.mu.Unlock()
		err := c.writer.Close()
		c.mu.Lock()
		defer c.mu.Unlock()
		c.closeErr = errors.Join(c.err, err)
	})
	return c.closeErr
}
