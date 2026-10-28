package telemetry

import "time"

// Signal timestamps identify host response completion/receipt, not ECU capture.
type Signal struct {
	Value        *float64  `json:"value"`
	Unit         string    `json:"unit"`
	Source       string    `json:"source"`
	Support      string    `json:"support"`
	Status       string    `json:"status"`
	CapturedAt   time.Time `json:"captured_at"`
	ReceivedAt   time.Time `json:"received_at"`
	StaleAfterMS int64     `json:"stale_after_ms"`
}

func freshTimes(now time.Time, ttl time.Duration, stamps ...time.Time) bool {
	if ttl <= 0 {
		return false
	}
	for _, stamp := range stamps {
		age := now.Sub(stamp)
		if stamp.IsZero() || age < 0 || age >= ttl {
			return false
		}
	}
	return true
}

func copyNumber(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return new(*value)
}

func (s Snapshot) Clone() Snapshot {
	s.RPM, s.SpeedKMH = copyNumber(s.RPM), copyNumber(s.SpeedKMH)
	s.CoolantC, s.VoltageV = copyNumber(s.CoolantC), copyNumber(s.VoltageV)
	if s.Signals != nil {
		signals := make(map[string]Signal, len(s.Signals))
		for key, signal := range s.Signals {
			signal.Value = copyNumber(signal.Value)
			signals[key] = signal
		}
		s.Signals = signals
	}
	return s
}

func (s Snapshot) signalsAt(now time.Time) Snapshot {
	s = s.Clone()
	for key, signal := range s.Signals {
		fresh := s.Health == Connected && signal.Status == "available" && freshTimes(now, time.Duration(signal.StaleAfterMS)*time.Millisecond, signal.CapturedAt, signal.ReceivedAt)
		if !fresh {
			signal.Value = nil
			if signal.Status == "available" {
				signal.Status = "stale"
			}
		}
		s.Signals[key] = signal
	}
	s.RPM = s.Signals["rpm"].Value
	s.SpeedKMH = s.Signals["speed_kmh"].Value
	s.CoolantC = s.Signals["coolant_c"].Value
	s.VoltageV = s.Signals["voltage_v"].Value
	return s
}
