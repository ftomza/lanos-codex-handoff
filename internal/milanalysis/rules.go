package milanalysis

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type point struct {
	value    float64
	at       time.Time
	evidence Evidence
}
type evaluator struct {
	findings []Finding
	normal   map[string][]Evidence
	seen     map[string]time.Time
	last     map[string]point
	series   map[string]series
	adequate map[string]bool
	checks   map[string]bool
	mapCount int
	previous *frameRecord
	polls    map[string]bool
	gaps     bool
}

func evaluate(frames []frameRecord) ([]Finding, []RuleCheck, bool) {
	e := evaluator{findings: []Finding{}, normal: make(map[string][]Evidence), seen: make(map[string]time.Time), last: make(map[string]point), series: make(map[string]series), adequate: make(map[string]bool), checks: make(map[string]bool), polls: make(map[string]bool)}
	for n := range frames {
		e.observe(frames[n])
	}
	for n := range e.findings {
		e.findings[n].Contradicting = e.normal[e.findings[n].Code]
		if e.findings[n].Contradicting == nil {
			e.findings[n].Contradicting = []Evidence{}
		}
	}
	slices.SortFunc(e.findings, func(a, b Finding) int { return strings.Compare(a.Code, b.Code) })
	checks := make([]RuleCheck, 0, 8)
	for _, code := range []string{"iat_out_of_range", "iat_jump", "iat_constant", "map_out_of_range", "map_context_mismatch", "fuel_trim_extreme", "o2_out_of_range", "o2_constant", "coolant_high", "coolant_jump", "adapter_voltage_low", "adapter_voltage_drop"} {
		status := "insufficient_data"
		detail := "Недостаточно свежих независимых измерений для оценки правила."
		if e.checks[code] || e.adequate[code] {
			status = "evaluated"
			detail = "Проверены доступные измерения; отсутствие находки не доказывает исправность."
		}
		checks = append(checks, RuleCheck{Code: code, Status: status, Detail: detail})
	}
	return e.findings, checks, e.gaps
}

func (e *evaluator) observe(f frameRecord) {
	if e.previous != nil {
		p := *e.previous
		if f.frame.At.Sub(p.frame.At) > 2*time.Second {
			e.gaps = true
			e.flag("telemetry_gap", "Шаг сохранённых кадров превышает 2 секунды.", "observed", Evidence{Reference: f.ref, Detail: fmt.Sprintf("%.3f s", f.frame.At.Sub(p.frame.At).Seconds())})
		}
		if f.ref.Generation != p.ref.Generation || f.segment != p.segment || f.frame.At.Sub(p.frame.At) > 2*time.Second {
			e.reset()
		}
	}
	e.quality(f)
	points := make(map[string]point, len(f.frame.Snapshot.Signals))
	fresh := make(map[string]bool, len(f.frame.Snapshot.Signals))
	keys := make([]string, 0, len(f.frame.Snapshot.Signals))
	for k := range f.frame.Snapshot.Signals {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if signalUnit(k) == "" {
			continue
		}
		s := f.frame.Snapshot.Signals[k]
		if !applicable(f, k, s) {
			delete(e.last, k)
			delete(e.series, k)
			if s.Value != nil && s.Status == "available" && s.Support != "unsupported" {
				e.flag("signal_unavailable", "Сохранённое числовое значение неприменимо: свежесть, статус, единицы или источник.", "observed", Evidence{Reference: f.ref, Signal: k, CapturedAt: s.CapturedAt, Value: s.Value, Unit: s.Unit})
			}
			continue
		}
		p := point{value: *s.Value, at: s.CapturedAt, evidence: Evidence{Reference: f.ref, Signal: k, CapturedAt: s.CapturedAt, Value: new(*s.Value), Unit: s.Unit}}
		points[k] = p
		captureKey := fmt.Sprintf("%d/%s", f.ref.Generation, k)
		if old, ok := e.seen[captureKey]; ok && !p.at.After(old) {
			continue
		}
		e.seen[captureKey] = p.at
		fresh[k] = true
	}
	e.ranges(points, fresh)
	e.temporal(points, fresh)
	e.mapContext(points, fresh)
	for k, p := range points {
		if fresh[k] {
			e.last[k] = p
		}
	}
	e.previous = &f
}

func (e *evaluator) reset() {
	e.last = make(map[string]point)
	e.series = make(map[string]series)
	e.mapCount = 0
}

func applicable(f frameRecord, k string, s telemetry.Signal) bool {
	if s.Support != "supported" {
		return false
	}
	if f.frame.EffectiveHealth != telemetry.Connected || f.frame.Snapshot.Health != telemetry.Connected || !f.frame.SignalAges[k].Applicable || s.Status != "available" || s.Value == nil || math.IsNaN(*s.Value) || math.IsInf(*s.Value, 0) || s.StaleAfterMS <= 0 {
		return false
	}
	unit := signalUnit(k)
	if unit == "" || s.Unit != unit {
		return false
	}
	if k == "voltage_v" {
		if s.Source != "adapter" && s.Source != "adapter_atrv" {
			return false
		}
	} else if s.Source != "ecu_mode01" {
		return false
	}
	for _, t := range []time.Time{s.CapturedAt, s.ReceivedAt} {
		age := f.frame.At.Sub(t)
		if t.IsZero() || age < 0 || age >= time.Duration(s.StaleAfterMS)*time.Millisecond {
			return false
		}
	}
	return true
}

func signalUnit(k string) string {
	switch k {
	case "rpm":
		return "rpm"
	case "speed_kmh":
		return "km/h"
	case "iat_c", "coolant_c":
		return "degC"
	case "map_kpa":
		return "kPa"
	case "load_percent", "throttle_percent", "stft_b1_percent", "ltft_b1_percent", "stft_b2_percent", "ltft_b2_percent":
		return "%"
	case "voltage_v":
		return "V"
	}
	if strings.HasPrefix(k, "o2_sensor") && strings.HasSuffix(k, "_v") {
		return "V"
	}
	return ""
}

func (e *evaluator) flag(code, symptom, confidence string, ev Evidence) {
	addFinding(&e.findings, code, symptom, confidence, ev, []string{"Независимая проверка причины наблюдения; источник аномалии не установлен."}, []string{"Сопоставить с исходными измерениями и проверить датчик/условия отдельно, сохранив коды."})
}

func (e *evaluator) rangeCheck(code, symptom string, p point, bad bool) {
	e.checks[code] = true
	if bad {
		e.flag(code, symptom, "observed", p.evidence)
		return
	}
	if len(e.normal[code]) < maxExamples {
		ev := p.evidence
		ev.Detail = "Другие свежие значения в нормальном диапазоне; это не отменяет отдельную аномалию."
		e.normal[code] = append(e.normal[code], ev)
	}
}
