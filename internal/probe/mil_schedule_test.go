package probe

import (
	"context"
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func milReport(now time.Time, on bool) Report {
	var report Report
	resetReport(&report, Config{}, 1)
	for _, pid := range []byte{1, 3, 0x0C} {
		report.Capabilities[int(pid)-1].Support = "supported"
	}
	c := &report.Capabilities[0]
	c.Status, c.CapturedAt, c.MIL = "available", now, &on
	return report
}

func TestMILIntervalValidation(t *testing.T) {
	t.Parallel()
	for _, interval := range []time.Duration{-1, time.Millisecond, 61 * time.Second} {
		_, err := New(Config{MILInterval: interval}, func(context.Context) (elm327.Transport, error) { return nil, nil })
		if err == nil {
			t.Errorf("accepted interval %v", interval)
		}
	}
	for _, interval := range []time.Duration{0, time.Second, 60 * time.Second} {
		if _, err := New(Config{MILInterval: interval}, func(context.Context) (elm327.Transport, error) { return nil, nil }); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMILSchedulePriorityFairnessBoundedAndBitmapGated(t *testing.T) {
	t.Parallel()
	now := time.Now()
	report := milReport(now, true)
	s := newPollSchedule(report, now, true)
	s.enableMIL(report, now, time.Second)
	var jobs, ordinary []string
	for i := 0; i < 60; i++ {
		now = now.Add(time.Second)
		if got := s.milCommand(now); got != "0101" {
			t.Fatalf("MIL not prioritized: %s", got)
		}
		s.complete("0101", now)
		s.observeMIL(report, now)
		command := s.support(now)
		if command == "" {
			continue
		}
		if s.isDiagnostic(command) {
			jobs = append(jobs, command)
			pids := []byte(nil)
			if command == "020000" {
				pids = []byte{2, 0x0C, 0x20}
			}
			s.completeDiagnostic(command, "available", pids)
		} else {
			ordinary = append(ordinary, command)
			s.complete(command, now)
		}
	}
	if !slices.Equal(jobs, []string{"03", "07", "0A", "020000", "020200", "020C00"}) {
		t.Fatalf("unbounded or ungated jobs: %v", jobs)
	}
	if !slices.Contains(ordinary, "0103") || !slices.Contains(ordinary, "ATRV") {
		t.Fatalf("support starved: %v", ordinary)
	}
}

func TestMILScheduleDeduplicatesAndLatchesUnsupportedPerGeneration(t *testing.T) {
	t.Parallel()
	now := time.Now()
	report := milReport(now, true)
	s := newPollSchedule(report, now, true)
	s.enableMIL(report, now, 5*time.Second)
	for _, command := range []string{"03", "07", "0A", "020000"} {
		s.completeDiagnostic(command, "unsupported", nil)
	}
	for i := 0; i < 100; i++ {
		s.observeMIL(report, now)
	}
	if len(s.mil.queue) != 0 {
		t.Fatalf("repeated ON queued jobs: %v", s.mil.queue)
	}
	report.Capabilities[0].MIL = new(bool)
	s.observeMIL(report, now)
	on := true
	report.Capabilities[0].MIL = &on
	s.observeMIL(report, now)
	if len(s.mil.queue) != 0 {
		t.Fatalf("unsupported repeated: %v", s.mil.queue)
	}
	fresh := newPollSchedule(report, now, true)
	fresh.enableMIL(report, now, 5*time.Second)
	if len(fresh.mil.queue) != 4 {
		t.Fatalf("generation did not reset: %v", fresh.mil.queue)
	}
}

func TestMILScheduleLossRearmsWithoutInventingOff(t *testing.T) {
	t.Parallel()
	now := time.Now()
	report := milReport(now, true)
	s := newPollSchedule(report, now, true)
	s.enableMIL(report, now, 5*time.Second)
	for _, command := range []string{"03", "07", "0A", "020000"} {
		s.completeDiagnostic(command, "available", nil)
	}
	report.Capabilities[0].Status = "temporarily_unavailable"
	s.observeMIL(report, now)
	if len(s.mil.queue) != 0 {
		t.Fatal("loss itself queued a transition")
	}
	report.Capabilities[0].Status = "available"
	s.observeMIL(report, now)
	if len(s.mil.queue) != 4 {
		t.Fatal("ON after gap did not refresh")
	}
	for _, command := range []string{"03", "07", "0A", "020000"} {
		s.completeDiagnostic(command, "available", nil)
	}
	s.observeMIL(report, now.Add(16*time.Second))
	report.Capabilities[0].CapturedAt = now.Add(17 * time.Second)
	s.observeMIL(report, now.Add(17*time.Second))
	if len(s.mil.queue) != 4 {
		t.Fatal("ON after expired data did not refresh")
	}
}

func TestMILTransitionRefreshesActiveJobWithoutQueueGrowth(t *testing.T) {
	t.Parallel()
	now := time.Now()
	report := milReport(now, true)
	s := newPollSchedule(report, now, true)
	s.enableMIL(report, now, time.Second)
	for i := 0; i < 100; i++ {
		s.completeDiagnostic("03", "available", nil)
		off := false
		report.Capabilities[0].MIL = &off
		s.observeMIL(report, now)
		on := true
		report.Capabilities[0].MIL = &on
		s.observeMIL(report, now)
		if len(s.mil.queue) != 4 || s.mil.queue[0] != "03" {
			t.Fatalf("transition not refreshed/bounded: %v", s.mil.queue)
		}
	}
}

func TestMILScheduleLatchesDiscoveryUnsupported(t *testing.T) {
	t.Parallel()
	now := time.Now()
	report := milReport(now, true)
	report.StoredDTCs.Status = "unsupported"
	s := newPollSchedule(report, now, true)
	s.enableMIL(report, now, time.Second)
	if slices.Contains(s.mil.queue, "03") {
		t.Fatalf("discovery rejection repeated: %v", s.mil.queue)
	}
	report.Capabilities[0].Status = "unsupported"
	s = newPollSchedule(report, now, true)
	s.enableMIL(report, now, time.Second)
	if command := s.milCommand(now.Add(time.Hour)); command != "" {
		t.Fatalf("discovery rejection repeated: %s", command)
	}
}
