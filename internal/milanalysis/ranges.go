package milanalysis

import (
	"math"
	"strings"
	"time"
)

func (e *evaluator) ranges(p map[string]point, fresh map[string]bool) {
	rpm, running := p["rpm"]
	running = running && rpm.value > 0
	if v, ok := p["iat_c"]; ok && fresh["iat_c"] {
		e.rangeCheck("iat_out_of_range", "IAT вне рабочего диапазона экспериментальной модели топлива.", v, v.value <= -30 || v.value > 100)
	}
	if v, ok := p["map_kpa"]; ok && fresh["map_kpa"] && running {
		e.rangeCheck("map_out_of_range", "MAP вне ожидаемого диапазона при работающем двигателе.", v, v.value <= 0 || v.value > 110)
	}
	if v, ok := p["coolant_c"]; ok && fresh["coolant_c"] {
		e.rangeCheck("coolant_high", "ECT достигла 110 °C или выше.", v, v.value >= 110)
	}
	if v, ok := p["voltage_v"]; ok && fresh["voltage_v"] && running && rpm.value >= 600 {
		e.rangeCheck("adapter_voltage_low", "Питание адаптера ниже 12 V при работающем двигателе; это не диагноз генератора.", v, v.value < 12)
	}
	for _, k := range []string{"stft_b1_percent", "ltft_b1_percent", "stft_b2_percent", "ltft_b2_percent"} {
		if v, ok := p[k]; ok && fresh[k] {
			e.rangeCheck("fuel_trim_extreme", "Модуль STFT/LTFT не менее 20%; разовые наблюдения не устанавливают причину.", v, math.Abs(v.value) >= 20)
		}
	}
	// Iterate a fixed sensor order for deterministic evidence aggregation.
	for _, k := range oxygenKeys {
		if v, ok := p[k]; ok && fresh[k] {
			e.rangeCheck("o2_out_of_range", "Напряжение narrowband O2 вне кодируемого диапазона 0..1.275 V.", v, v.value < 0 || v.value > 1.275)
		}
	}
}

var oxygenKeys = []string{"o2_sensor1_v", "o2_sensor2_v", "o2_sensor3_v", "o2_sensor4_v", "o2_sensor5_v", "o2_sensor6_v", "o2_sensor7_v", "o2_sensor8_v"}

func (e *evaluator) mapContext(p map[string]point, fresh map[string]bool) {
	keys := []string{"map_kpa", "rpm", "load_percent", "throttle_percent"}
	var earliest, latest time.Time
	for _, k := range keys {
		v, ok := p[k]
		if !ok {
			e.mapCount = 0
			return
		}
		if earliest.IsZero() || v.at.Before(earliest) {
			earliest = v.at
		}
		if v.at.After(latest) {
			latest = v.at
		}
	}
	if latest.Sub(earliest) > 2*time.Second {
		e.mapCount = 0
		return
	}
	if !fresh["map_kpa"] {
		return
	}
	e.checks["map_context_mismatch"] = true
	bad := p["map_kpa"].value >= 95 && p["rpm"].value >= 600 && p["rpm"].value <= 1200 && p["load_percent"].value <= 20 && p["throttle_percent"].value <= 5
	if !bad {
		e.mapCount = 0
		return
	}
	e.mapCount++
	if e.mapCount >= 3 {
		ev := p["map_kpa"].evidence
		ev.Detail = "Согласованные по времени RPM/load/throttle: " + formatPoints(p, keys[1:])
		e.flag("map_context_mismatch", "Высокий MAP при малой нагрузке/дросселе и низких RPM; проверить согласованность условий.", "limited_hypothesis", ev)
	}
}

func formatPoints(p map[string]point, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+number(p[k].value))
	}
	return strings.Join(parts, ", ")
}
