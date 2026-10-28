package probe

import (
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

type scheduledItem struct {
	Command    string        `json:"command"`
	IntervalNS time.Duration `json:"interval_ns"`
	due        time.Time
}

func supportingItems() []scheduledItem {
	return []scheduledItem{
		{Command: "0103", IntervalNS: 3 * time.Second},
		{Command: "0111", IntervalNS: 3 * time.Second},
		{Command: "0106", IntervalNS: 5 * time.Second},
		{Command: "010F", IntervalNS: 5 * time.Second},
		{Command: "0104", IntervalNS: 10 * time.Second},
		{Command: "0105", IntervalNS: 10 * time.Second},
		{Command: "010E", IntervalNS: 10 * time.Second},
		{Command: "0114", IntervalNS: 10 * time.Second},
		{Command: "0115", IntervalNS: 10 * time.Second},
		{Command: "0107", IntervalNS: 30 * time.Second},
		{Command: "0101", IntervalNS: 60 * time.Second},
		{Command: "ATRV", IntervalNS: 60 * time.Second},
	}
}

type pollSchedule struct {
	core  []byte
	items []scheduledItem
}

func newPollSchedule(report Report, now time.Time, includeFuel bool) *pollSchedule {
	s := &pollSchedule{core: make([]byte, 0, 5), items: supportingItems()}
	pids := []byte{0x0C, 0x0B, 0x0D}
	if includeFuel {
		pids = append(pids, 0x10, 0x5E)
	}
	for _, pid := range pids {
		if report.Capabilities[int(pid)-1].Support == "supported" {
			s.core = append(s.core, pid)
		}
	}
	s.items = slices.DeleteFunc(s.items, func(item scheduledItem) bool {
		return item.Command != "ATRV" && report.Capabilities[int(itemPID(item.Command))-1].Support != "supported"
	})
	for i := range s.items {
		item := &s.items[i]
		captured := report.AdapterVoltage.CapturedAt
		if item.Command != "ATRV" {
			captured = report.Capabilities[int(itemPID(item.Command))-1].CapturedAt
		}
		delay := time.Duration(0)
		if !captured.IsZero() {
			delay = max(0, item.IntervalNS-now.Sub(captured))
		}
		item.due = now.Add(delay)
	}
	return s
}

func itemPID(command string) byte {
	var value byte
	for _, c := range command[2:] {
		value <<= 4
		if c >= 'A' {
			value += byte(c - 'A' + 10)
			continue
		}
		value += byte(c - '0')
	}
	return value
}

func (s *pollSchedule) support(now time.Time) string {
	var selected *scheduledItem
	for i := range s.items {
		item := &s.items[i]
		if item.due.After(now) {
			continue
		}
		if selected == nil || item.due.Before(selected.due) || item.due.Equal(selected.due) && item.Command < selected.Command {
			selected = item
		}
	}
	if selected == nil {
		return ""
	}
	return selected.Command
}

func (s *pollSchedule) complete(command string, completed time.Time) {
	for i := range s.items {
		if s.items[i].Command == command {
			s.items[i].due = completed.Add(s.items[i].IntervalNS)
			return
		}
	}
}

func missingCore(report Report) []string {
	missing := make([]string, 0, 4)
	for _, pid := range []byte{0x0C, 0x0B, 0x0D, 0x0F} {
		c := report.Capabilities[int(pid)-1]
		if c.Support != "supported" || c.Status != "available" {
			missing = append(missing, elm327.HexPID(pid))
		}
	}
	return missing
}
