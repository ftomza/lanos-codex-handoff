package mil

import (
	"encoding/json"
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

const PreWindow = 60 * time.Second
const PostWindow = 30 * time.Second
const MaxFrames = 256

type SignalAge struct {
	CapturedMS *int64 `json:"captured_ms"`
	ReceivedMS *int64 `json:"received_ms"`
	Applicable bool   `json:"applicable"`
}

// Snapshot is raw evidence. SignalAges indicates applicability at this frame's At.
// FuelContext has its own input timestamps; sequential sources are not simultaneous.
type Frame struct {
	At          time.Time            `json:"at"`
	Snapshot    telemetry.Snapshot   `json:"snapshot"`
	SignalAges  map[string]SignalAge `json:"signal_ages,omitempty"`
	FuelContext json.RawMessage      `json:"fuel_context,omitempty"`
}

func NewFrame(now time.Time, snapshot telemetry.Snapshot, staleAfter time.Duration) Frame {
	f := Frame{At: now.UTC(), Snapshot: snapshot.Clone(), SignalAges: make(map[string]SignalAge, len(snapshot.Signals))}
	effective := snapshot.At(now, staleAfter)
	for k, s := range snapshot.Signals {
		age := SignalAge{Applicable: effective.Signals[k].Status == "available" && effective.Signals[k].Value != nil}
		if !s.CapturedAt.IsZero() {
			age.CapturedMS = new(now.Sub(s.CapturedAt).Milliseconds())
		}
		if !s.ReceivedAt.IsZero() {
			age.ReceivedMS = new(now.Sub(s.ReceivedAt).Milliseconds())
		}
		f.SignalAges[k] = age
	}
	return f
}

func (f Frame) clone() Frame {
	f.Snapshot = f.Snapshot.Clone()
	f.FuelContext = slices.Clone(f.FuelContext)
	ages := make(map[string]SignalAge, len(f.SignalAges))
	for key, age := range f.SignalAges {
		if age.CapturedMS != nil {
			age.CapturedMS = new(*age.CapturedMS)
		}
		if age.ReceivedMS != nil {
			age.ReceivedMS = new(*age.ReceivedMS)
		}
		ages[key] = age
	}
	f.SignalAges = ages
	return f
}

type Ring struct {
	frames []Frame
	last   time.Time
}

func NewRing() *Ring { return &Ring{frames: make([]Frame, 0, MaxFrames)} }

func (r *Ring) Add(f Frame, force bool) bool {
	if f.At.IsZero() || !r.last.IsZero() && f.At.Before(r.last) {
		return false
	}
	if !force && !r.last.IsZero() && f.At.Sub(r.last) < time.Second {
		return false
	}
	r.last = f.At
	// Retain one predecessor at the window boundary: samples are discrete,
	// whereas the event timestamp may fall between two samples.
	cutoff := f.At.Add(-PreWindow)
	remove := 0
	for remove+1 < len(r.frames) && !r.frames[remove+1].At.After(cutoff) {
		remove++
	}
	r.frames = slices.Delete(r.frames, 0, remove)
	if len(r.frames) >= MaxFrames {
		copy(r.frames, r.frames[1:])
		r.frames = r.frames[:len(r.frames)-1]
	}
	r.frames = append(r.frames, f.clone())
	return true
}

func (r *Ring) Before(at time.Time) ([]Frame, bool) {
	frames := make([]Frame, 0, len(r.frames))
	start := 0
	for i, frame := range r.frames {
		if !frame.At.After(at.Add(-PreWindow)) {
			start = i
		}
	}
	for _, frame := range r.frames[start:] {
		if !frame.At.After(at) {
			frames = append(frames, frame.clone())
		}
	}
	complete := len(frames) > 0 && !frames[0].At.After(at.Add(-PreWindow))
	return frames, complete
}
