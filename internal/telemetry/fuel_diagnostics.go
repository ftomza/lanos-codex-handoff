package telemetry

// SpeedDensityTerms are the actual estimator intermediates, with explicit units.
// Nil identifies a term that was not computed (including the stopped-engine path).
type SpeedDensityTerms struct {
	PressurePa      *float64 `json:"pressure_pa"`
	TemperatureK    *float64 `json:"temperature_k"`
	DensityKGM3     *float64 `json:"air_density_kg_m3"`
	VolumeM3S       *float64 `json:"volume_m3_s"`
	AirGS           *float64 `json:"air_g_s"`
	RawFlowLPH      *float64 `json:"raw_flow_lph"`
	AdjustedFlowLPH *float64 `json:"adjusted_flow_lph"`
	VE              float64  `json:"ve"`
	AFR             float64  `json:"afr"`
	FuelDensityGL   float64  `json:"fuel_density_g_l"`
	Factor          float64  `json:"factor"`
	EngineStopped   bool     `json:"engine_stopped"`
}

func (v *SpeedDensityTerms) Clone() *SpeedDensityTerms {
	if v == nil {
		return nil
	}
	out := *v
	out.PressurePa = copyNumber(v.PressurePa)
	out.TemperatureK = copyNumber(v.TemperatureK)
	out.DensityKGM3 = copyNumber(v.DensityKGM3)
	out.VolumeM3S = copyNumber(v.VolumeM3S)
	out.AirGS = copyNumber(v.AirGS)
	out.RawFlowLPH = copyNumber(v.RawFlowLPH)
	out.AdjustedFlowLPH = copyNumber(v.AdjustedFlowLPH)
	return &out
}
