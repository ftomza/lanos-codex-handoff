package probe

import (
	"testing"
	"time"
)

func TestSupportingScheduleUsesOldestDueAndCompletion(t *testing.T) {
	t.Parallel()
	now := time.Now()
	var report Report
	resetReport(&report, Config{}, 1)
	for _, pid := range []byte{3, 0x11, 0x0C, 0x0D} {
		report.Capabilities[int(pid)-1].Support = "supported"
		report.Capabilities[int(pid)-1].CapturedAt = now
	}
	report.AdapterVoltage.CapturedAt = now
	s := newPollSchedule(report, now)
	if c := s.support(now); c != "" {
		t.Fatalf("polled early: %s", c)
	}
	now = now.Add(20 * time.Second)
	if c := s.support(now); c != "0103" {
		t.Fatalf("tie: %s", c)
	}
	s.complete("0103", now)
	if c := s.support(now); c != "0111" {
		t.Fatalf("starved older work: %s", c)
	}
	s.complete("0111", now)
	if c := s.support(now.Add(time.Second)); c != "" {
		t.Fatalf("catch-up burst: %s", c)
	}
	if len(s.core) != 2 || s.core[0] != 0x0C || s.core[1] != 0x0D {
		t.Fatalf("unadvertised MAP: %v", s.core)
	}
}

func TestRequiredInputsIncludeAdvertisedButUnavailableReadings(t *testing.T) {
	t.Parallel()
	var report Report
	resetReport(&report, Config{}, 1)
	for _, pid := range []byte{0x0C, 0x0B, 0x0D, 0x0F} {
		c := &report.Capabilities[int(pid)-1]
		c.Support, c.Status = "supported", "available"
	}
	report.Capabilities[0x0B-1].Status = "temporarily_unavailable"
	missing := missingCore(report)
	if len(missing) != 1 || missing[0] != "0B" {
		t.Fatalf("advertised support presented as a successful measurement: %v", missing)
	}
}
