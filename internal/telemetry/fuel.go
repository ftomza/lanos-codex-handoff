package telemetry

import "time"

// Fuel contains derived estimates and historical since-reset totals, never tank level.
type Fuel struct {
	Quality          string             `json:"quality,omitempty"`
	QualityReasons   []string           `json:"quality_reasons,omitempty"`
	ColdStartCheck   string             `json:"cold_start_check,omitempty"`
	Intermediates    *SpeedDensityTerms `json:"intermediates,omitempty"`
	DiagnosticsError string             `json:"diagnostics_error,omitempty"`
	Status           string             `json:"status"`
	Reason           string             `json:"reason"`
	Method           string             `json:"method"`
	Provenance       string             `json:"provenance"`
	Confidence       string             `json:"confidence"`
	FlowLPH          *float64           `json:"flow_lph"`
	InstantL100      *float64           `json:"instant_l100"`
	FlowUntil        time.Time          `json:"flow_until"`
	SpeedUntil       time.Time          `json:"speed_until"`
	LowSpeed         float64            `json:"low_speed_kmh"`
	Speed            *float64           `json:"speed_kmh"`
	Liters           float64            `json:"liters"`
	DistanceKM       float64            `json:"distance_km"`
	CoveredKM        float64            `json:"covered_km"`
	AverageL100      *float64           `json:"average_l100"`
	ObservedSeconds  float64            `json:"observed_seconds"`
	CoveredSeconds   float64            `json:"covered_seconds"`
	GapSeconds       float64            `json:"gap_seconds"`
	Incomplete       bool               `json:"incomplete"`
	CoverageReasons  []string           `json:"coverage_reasons"`
	ResetAt          time.Time          `json:"reset_at"`
	SavedAt          time.Time          `json:"saved_at"`
	Factor           float64            `json:"factor"`
	PreviousFactor   *float64           `json:"previous_factor"`
	Revision         uint64             `json:"revision"`
	Persistence      string             `json:"persistence"`
	PersistenceError string             `json:"persistence_error,omitzero"`
	Settings         FuelSettings       `json:"settings"`
}

type FuelSettings struct {
	Displacement  float64 `json:"displacement_l"`
	VE            float64 `json:"ve"`
	AFR           float64 `json:"afr"`
	Density       float64 `json:"density_g_l"`
	UnverifiedIAT bool    `json:"unverified_iat"`
}

func (f Fuel) At(now time.Time, health Health) Fuel {
	f.FlowLPH, f.InstantL100, f.Speed = copyNumber(f.FlowLPH), copyNumber(f.InstantL100), copyNumber(f.Speed)
	f.AverageL100, f.PreviousFactor = copyNumber(f.AverageL100), copyNumber(f.PreviousFactor)
	f.Intermediates = f.Intermediates.Clone()
	f.QualityReasons = append([]string{}, f.QualityReasons...)
	f.CoverageReasons = append([]string{}, f.CoverageReasons...)
	if health != Connected || f.FlowUntil.IsZero() || !now.Before(f.FlowUntil) {
		f.FlowLPH, f.InstantL100 = nil, nil
		f.Quality = "UNAVAILABLE"
		f.Intermediates = nil
		if f.Status == "available" {
			f.Status, f.Reason = "unavailable", "expired"
		}
	}
	if health != Connected || f.SpeedUntil.IsZero() || !now.Before(f.SpeedUntil) {
		f.Speed, f.InstantL100 = nil, nil
	}
	return f
}
