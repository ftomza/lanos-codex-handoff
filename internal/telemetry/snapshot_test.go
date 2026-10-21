package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSnapshotMissingMeasurementsAreNull(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(Snapshot{Source: Simulator, Health: Disconnected})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"rpm", "speed_kmh", "coolant_c", "voltage_v"} {
		if !strings.Contains(string(data), `"`+key+`":null`) {
			t.Errorf("missing %s must serialize as null: %s", key, data)
		}
	}
}

func TestSnapshotFreshness(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name               string
		health             Health
		captured, received time.Time
		want               Health
	}{
		{"fresh", Connected, now, now, Connected},
		{"old capture", Connected, now.Add(-4 * time.Second), now, Stale},
		{"old receipt", Connected, now, now.Add(-4 * time.Second), Stale},
		{"missing timestamps", Connected, time.Time{}, time.Time{}, Stale},
		{"future capture", Connected, now.Add(time.Second), now, Stale},
		{"disconnect stays disconnected", Disconnected, now.Add(-time.Hour), now, Disconnected},
		{"starting", Connecting, time.Time{}, time.Time{}, Connecting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := Snapshot{Health: tc.health, CapturedAt: tc.captured, ReceivedAt: tc.received}
			if got := s.At(now, 3*time.Second).Health; got != tc.want {
				t.Fatalf("health = %s, want %s", got, tc.want)
			}
			if s.Health != tc.health {
				t.Fatal("freshness must not mutate the stored snapshot")
			}
		})
	}
}
