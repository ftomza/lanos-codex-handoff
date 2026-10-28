package fuelhistory

import "time"

type Stats struct {
	SpeedSeconds    float64  `json:"speed_seconds"`
	SpeedDistanceKM float64  `json:"speed_distance_km"`
	Seconds         float64  `json:"seconds"`
	Liters          float64  `json:"liters"`
	DistanceKM      float64  `json:"distance_km"`
	JointLiters     float64  `json:"joint_liters"`
	CoveredKM       float64  `json:"covered_km"`
	CoveredSeconds  float64  `json:"covered_seconds"`
	AverageL100     *float64 `json:"average_l100"`
	AverageSpeed    *float64 `json:"average_speed_kmh"`
}

func (s *Stats) finish() {
	if s.CoveredKM >= .1 {
		s.AverageL100 = new(100 * s.JointLiters / s.CoveredKM)
	}
	if s.SpeedSeconds > 0 {
		s.AverageSpeed = new(s.SpeedDistanceKM * 3600 / s.SpeedSeconds)
	}
}
func (s *Stats) add(v Stats) {
	s.SpeedSeconds += v.SpeedSeconds
	s.SpeedDistanceKM += v.SpeedDistanceKM
	s.Seconds += v.Seconds
	s.Liters += v.Liters
	s.DistanceKM += v.DistanceKM
	s.JointLiters += v.JointLiters
	s.CoveredKM += v.CoveredKM
	s.CoveredSeconds += v.CoveredSeconds
}

type Bin struct {
	Label string `json:"label"`
	Stats
}
type Segment struct {
	Session string    `json:"session_id"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Reason  string    `json:"reason"`
	Stats
}
type Point struct {
	At    time.Time `json:"at"`
	Value *float64  `json:"value"`
	Break bool      `json:"break,omitempty"`
}
type Marker struct {
	At     time.Time `json:"at"`
	Reason string    `json:"reason"`
}
type Anomaly struct {
	At   time.Time `json:"at"`
	Code string    `json:"code"`
}
type Report struct {
	Version int       `json:"version"`
	Source  string    `json:"source"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Stats
	DurationSeconds    float64            `json:"duration_seconds"`
	GapSeconds         float64            `json:"gap_seconds"`
	CoveragePercent    float64            `json:"coverage_percent"`
	SampledLiters      float64            `json:"sampled_liters"`
	MaxSpeed           *float64           `json:"max_speed_kmh"`
	Incomplete         bool               `json:"incomplete"`
	RetentionTruncated bool               `json:"retention_truncated"`
	Downsampled        bool               `json:"downsampled"`
	Samples            int64              `json:"samples"`
	Rows               int64              `json:"rows"`
	Issues             []Issue            `json:"issues"`
	Anomalies          []Anomaly          `json:"anomalies"`
	AnomalyCount       int64              `json:"anomaly_count"`
	Markers            []Marker           `json:"markers"`
	Modes              map[string]*Stats  `json:"modes"`
	Distributions      map[string][]Bin   `json:"distributions"`
	Segments           []Segment          `json:"segments"`
	Series             map[string][]Point `json:"series"`
	Last               *Entry             `json:"last,omitempty"`
	CoverageScope      string             `json:"coverage_scope"`
}
