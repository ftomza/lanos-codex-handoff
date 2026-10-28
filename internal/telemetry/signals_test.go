package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSignalsExpireIndependentlyAndDoNotMutateHistory(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	s := Snapshot{Source: Live, Health: Connected, CapturedAt: now, ReceivedAt: now, RPM: new(900.), CoolantC: new(80.), Signals: map[string]Signal{
		"rpm":       {Value: new(900.), Status: "available", Support: "supported", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
		"coolant_c": {Value: new(80.), Status: "available", Support: "supported", CapturedAt: now.Add(-36 * time.Second), ReceivedAt: now.Add(-36 * time.Second), StaleAfterMS: 35000},
	}}
	got := s.At(now, 3*time.Second)
	if got.Health != Connected || got.RPM == nil || got.CoolantC != nil || got.Signals["coolant_c"].Status != "stale" {
		t.Fatalf("expiry: %+v", got)
	}
	if s.CoolantC == nil || s.Signals["coolant_c"].Value == nil || s.Signals["coolant_c"].Status != "available" {
		t.Fatal("At mutated stored history")
	}
	copy := s.Clone()
	*copy.RPM = 1
	*copy.Signals["rpm"].Value = 2
	if *s.RPM != 900 || *s.Signals["rpm"].Value != 900 {
		t.Fatal("mutable clone aliases original")
	}
	data, err := json.Marshal(got)
	if err != nil || !strings.Contains(string(data), `"coolant_c":null`) {
		t.Fatalf("stale value serialized: %s %v", data, err)
	}
}

func TestLiveSilenceAndDisconnectHideAllNumbers(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	for _, health := range []Health{Connected, Disconnected, Connecting} {
		s := Snapshot{Source: Live, Health: health, RPM: new(800.), VoltageV: new(13.5), CapturedAt: now.Add(-4 * time.Second), ReceivedAt: now.Add(-4 * time.Second), Signals: map[string]Signal{
			"rpm":       {Value: new(800.), Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000},
			"voltage_v": {Value: new(13.5), Status: "available", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 130000},
		}}
		got := s.At(now, 3*time.Second)
		if got.RPM != nil || got.VoltageV != nil || got.Signals["voltage_v"].Value != nil {
			t.Fatalf("non-live values leaked: %+v", got)
		}
	}
}

func TestUnavailableSignalCannotPublishNumericValue(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	s := Snapshot{Source: Live, Health: Connected, CapturedAt: now, ReceivedAt: now, RPM: new(999.), Signals: map[string]Signal{"rpm": {Value: new(999.), Status: "temporarily_unavailable", CapturedAt: now, ReceivedAt: now, StaleAfterMS: 3000}}}
	got := s.At(now, 3*time.Second)
	if got.RPM != nil || got.Signals["rpm"].Value != nil {
		t.Fatal("NO DATA retained numeric value")
	}
}
