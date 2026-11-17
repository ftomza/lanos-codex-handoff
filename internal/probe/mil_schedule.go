package probe

import (
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

// milWork belongs to one connection generation. Its queue has at most four
// service requests plus the finite, allowlisted frame-00 context subset.
type milWork struct {
	interval     time.Duration
	known        bool
	on           bool
	queue        []string
	unsupported  map[string]bool
	ordinaryNext bool
}

func (s *pollSchedule) enableMIL(report Report, now time.Time, interval time.Duration) {
	if interval == 0 {
		return
	}
	s.mil = &milWork{interval: interval, unsupported: make(map[string]bool)}
	s.mil.unsupported["03"] = report.StoredDTCs.Status == "unsupported"
	s.mil.unsupported["0101"] = report.Capabilities[0].Status == "unsupported"
	for i := range s.items {
		if s.items[i].Command == "0101" {
			s.items[i].IntervalNS = interval
			captured := report.Capabilities[0].CapturedAt
			s.items[i].due = now
			if !captured.IsZero() {
				s.items[i].due = captured.Add(interval)
			}
		}
	}
	s.observeMIL(report, now)
}

func (s *pollSchedule) observeMIL(report Report, now time.Time) {
	if s.mil == nil {
		return
	}
	c := report.Capabilities[0]
	age := now.Sub(c.CapturedAt)
	if c.Status != "available" || c.MIL == nil || c.CapturedAt.IsZero() || age < 0 || age > max(15*time.Second, 3*s.mil.interval) {
		s.mil.known = false
		return
	}
	trigger := *c.MIL && (!s.mil.known || !s.mil.on)
	s.mil.known, s.mil.on = true, *c.MIL
	if trigger {
		s.mil.queue = nil
		for _, command := range []string{"03", "07", "0A", "020000"} {
			if !s.mil.unsupported[command] {
				s.mil.queue = append(s.mil.queue, command)
			}
		}
	}
}

func (s *pollSchedule) milCommand(now time.Time) string {
	if s.mil == nil {
		return ""
	}
	for _, item := range s.items {
		if item.Command == "0101" && !item.due.After(now) && !s.mil.unsupported["0101"] {
			return item.Command
		}
	}
	return ""
}

func (s *pollSchedule) isDiagnostic(command string) bool {
	return s.mil != nil && slices.Contains(s.mil.queue, command)
}

func (s *pollSchedule) completeDiagnostic(command, status string, pids []byte) {
	if s.mil == nil {
		return
	}
	s.mil.queue = slices.DeleteFunc(s.mil.queue, func(queued string) bool { return queued == command })
	if status == "unsupported" {
		s.mil.unsupported[command] = true
	}
	if command == "020000" && status == "available" {
		for _, pid := range elm327.FreezeFramePIDs() {
			candidate, _ := elm327.FreezeFrameCommand(pid)
			if slices.Contains(pids, pid) && !s.mil.unsupported[candidate] {
				s.mil.queue = append(s.mil.queue, candidate)
			}
		}
	}
	s.mil.ordinaryNext = true
}
