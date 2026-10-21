package sim

import (
	"math"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

// Sample is a pure, repeatable 60-second cycle: idle, accelerate to 80 km/h,
// cruise, decelerate, idle, lose connection at 48s, recover at 54s.
func Sample(start, now time.Time) telemetry.Snapshot {
	elapsed := max(0, now.Sub(start).Seconds())
	phase := math.Mod(elapsed, 60)
	s := telemetry.Snapshot{
		Source: telemetry.Simulator, Health: telemetry.Connected,
		CapturedAt: now.UTC(), ReceivedAt: now.UTC(),
		Message: "simulated driving cycle; no vehicle connected",
	}
	if phase >= 48 && phase < 54 {
		s.Health = telemetry.Disconnected
		s.CapturedAt = time.Time{}
		s.Message = "simulated connection loss; recovery at cycle second 54"
		return s
	}
	speed := 0.0
	switch {
	case phase >= 5 && phase < 25:
		speed = (phase - 5) * 4
	case phase >= 25 && phase < 35:
		speed = 80
	case phase >= 35 && phase < 45:
		speed = (45 - phase) * 8
	}
	rpm := 850 + speed*35 + 20*math.Sin(phase*2)
	coolant := 78 + 12*(1-math.Exp(-elapsed/90))
	s.RPM, s.SpeedKMH, s.CoolantC = &rpm, &speed, &coolant
	if phase < 24 || phase >= 28 {
		voltage := 14.1 + 0.12*math.Sin(phase/3)
		s.VoltageV = &voltage
	}
	return s
}
