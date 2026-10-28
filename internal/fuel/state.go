package fuel

import (
	"fmt"
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type State struct {
	Version        int       `json:"version"`
	Source         string    `json:"source"`
	Fingerprint    string    `json:"fingerprint"`
	Method         string    `json:"method"`
	Revision       uint64    `json:"revision"`
	ResetAt        time.Time `json:"reset_at"`
	SavedAt        time.Time `json:"saved_at"`
	BaseLiters     float64   `json:"base_liters"`
	JointLiters    float64   `json:"joint_liters"`
	DistanceKM     float64   `json:"distance_km"`
	CoveredKM      float64   `json:"covered_km"`
	ObservedNS     int64     `json:"observed_ns"`
	CoveredNS      int64     `json:"covered_ns"`
	GapNS          int64     `json:"gap_ns"`
	Incomplete     bool      `json:"incomplete"`
	Reasons        []string  `json:"reasons"`
	Factor         float64   `json:"factor"`
	PreviousFactor *float64  `json:"previous_factor"`
}

type Store interface {
	Load() (*State, error)
	Save(State) error
	Close() error
}

func (s State) clone() State {
	s.Reasons = slices.Clone(s.Reasons)
	if s.PreviousFactor != nil {
		s.PreviousFactor = new(*s.PreviousFactor)
	}
	return s
}
func (s State) validate() error {
	if s.Version != 1 || !slices.Contains([]string{telemetry.Simulator, telemetry.Live}, s.Source) || len(s.Fingerprint) != 64 || s.Revision == 0 || s.ResetAt.IsZero() || s.SavedAt.IsZero() {
		return fmt.Errorf("invalid fuel state metadata")
	}
	if !slices.Contains([]string{"", "direct", "maf", "speed_density"}, s.Method) || len(s.Reasons) > 16 {
		return fmt.Errorf("invalid fuel state method or coverage")
	}
	for _, v := range []float64{s.BaseLiters, s.JointLiters, s.DistanceKM, s.CoveredKM} {
		if !bounded(v, 0, 1e9) {
			return fmt.Errorf("invalid fuel state totals")
		}
	}
	if s.JointLiters > s.BaseLiters+1e-9 || s.CoveredKM > s.DistanceKM+1e-9 || s.ObservedNS < 0 || s.CoveredNS < 0 || s.GapNS < 0 || s.CoveredNS > s.ObservedNS || s.GapNS > s.ObservedNS {
		return fmt.Errorf("inconsistent fuel coverage")
	}
	if !bounded(s.Factor, .25, 4) || s.PreviousFactor != nil && !bounded(*s.PreviousFactor, .25, 4) {
		return fmt.Errorf("invalid fuel calibration factor")
	}
	return nil
}
func (s *State) mark(reason string) {
	s.Incomplete = true
	if !slices.Contains(s.Reasons, reason) {
		s.Reasons = append(s.Reasons, reason)
	}
}
