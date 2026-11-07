package milanalysis

import (
	"slices"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
)

func TestThresholdRulesRequireFreshCorrectUnits(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key           string
		value, normal float64
		code          string
	}{
		{"iat_c", -30, -29, "iat_out_of_range"}, {"iat_c", 101, 100, "iat_out_of_range"},
		{"map_kpa", 111, 110, "map_out_of_range"}, {"map_kpa", 0, 1, "map_out_of_range"},
		{"stft_b1_percent", 20, 19, "fuel_trim_extreme"}, {"ltft_b1_percent", -20, -19, "fuel_trim_extreme"},
		{"coolant_c", 110, 109, "coolant_high"}, {"voltage_v", 11.9, 12, "adapter_voltage_low"},
		{"o2_sensor1_v", 1.276, 1.275, "o2_out_of_range"},
	} {
		t.Run(tc.key+tc.code, func(t *testing.T) {
			t.Parallel()
			for _, abnormal := range []bool{true, false} {
				v := tc.normal
				if abnormal {
					v = tc.value
				}
				f := sample(epoch, map[string]float64{tc.key: v, "rpm": 900})
				r, err := Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f})), Options{})
				if err != nil {
					t.Fatal(err)
				}
				if slices.Contains(codes(r), tc.code) != abnormal {
					t.Fatalf("abnormal=%t codes=%v", abnormal, codes(r))
				}
				bad := f.Snapshot.Signals[tc.key]
				bad.Unit = "wrong"
				f.Snapshot.Signals[tc.key] = bad
				r, err = Analyze(t.Context(), archive(t, incidentRows(t, []mil.Frame{f})), Options{})
				if err != nil {
					t.Fatal(err)
				}
				if slices.Contains(codes(r), tc.code) {
					t.Fatal("used wrong units")
				}
			}
		})
	}
}

func TestTemporalRulesResetAtGapsAndGenerations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key, code string
		a, b      float64
		dt        time.Duration
	}{
		{"iat_c", "iat_jump", -40, 20, 10 * time.Second},
		{"coolant_c", "coolant_jump", 70, 85, 10 * time.Second},
		{"voltage_v", "adapter_voltage_drop", 14, 12.5, time.Second},
	} {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			frames := make([]mil.Frame, 0, int(tc.dt/time.Second)+1)
			for n := 0; n <= int(tc.dt/time.Second); n++ {
				f := sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{tc.key: tc.a, "rpm": 900})
				// Repeated snapshots refer to the original response until the last frame.
				s := f.Snapshot.Signals[tc.key]
				s.CapturedAt = epoch
				s.ReceivedAt = epoch
				if n == int(tc.dt/time.Second) {
					s.Value = new(tc.b)
					s.CapturedAt = f.At
					s.ReceivedAt = f.At
				}
				f.Snapshot.Signals[tc.key] = s
				frames = append(frames, f)
			}
			r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(codes(r), tc.code) {
				t.Fatal(codes(r))
			}
			frames[len(frames)-1].Snapshot.Generation = 2
			r, err = Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(codes(r), tc.code) {
				t.Fatal("spanned generations")
			}
		})
	}
}

func TestSparseO2AndShortIATDoNotBecomeSensorDiagnosis(t *testing.T) {
	t.Parallel()
	var frames []mil.Frame
	for n := range 61 {
		f := sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900, "coolant_c": 85, "iat_c": 20, "o2_sensor1_v": .8})
		s := f.Snapshot.Signals["o2_sensor1_v"]
		s.CapturedAt = epoch.Add(time.Duration(n/15) * 15 * time.Second)
		s.ReceivedAt = s.CapturedAt
		f.Snapshot.Signals["o2_sensor1_v"] = s
		frames = append(frames, f)
	}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(codes(r), "o2_constant") || slices.Contains(codes(r), "iat_constant") {
		t.Fatal(codes(r))
	}
	for _, c := range r.Sessions[0].Incidents[0].Checks {
		if (c.Code == "iat_constant" || c.Code == "o2_constant") && c.Status != "insufficient_data" {
			t.Fatal(c)
		}
	}
}

func TestSustainedFreshSignalsAreFlaggedWithoutCountingCachedSamples(t *testing.T) {
	t.Parallel()
	var frames []mil.Frame
	for n := range 311 {
		frames = append(frames, sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900, "coolant_c": 85, "iat_c": 20, "o2_sensor1_v": .8}))
	}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(codes(r), "iat_constant") || !slices.Contains(codes(r), "o2_constant") {
		t.Fatal(codes(r))
	}
}

func TestMAPContextNeedsMatchedCaptureTimes(t *testing.T) {
	t.Parallel()
	frames := []mil.Frame{}
	for n := range 3 {
		frames = append(frames, sample(epoch.Add(time.Duration(n)*time.Second), map[string]float64{"rpm": 900, "map_kpa": 98, "load_percent": 5, "throttle_percent": 0}))
	}
	r, err := Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(codes(r), "map_context_mismatch") {
		t.Fatal(codes(r))
	}
	for n := range frames {
		s := frames[n].Snapshot.Signals["load_percent"]
		s.CapturedAt = frames[n].At.Add(-3 * time.Second)
		s.ReceivedAt = s.CapturedAt
		frames[n].Snapshot.Signals["load_percent"] = s
	}
	r, err = Analyze(t.Context(), archive(t, incidentRows(t, frames)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(codes(r), "map_context_mismatch") {
		t.Fatal("used asynchronous inputs")
	}
}
