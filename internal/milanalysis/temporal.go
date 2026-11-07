package milanalysis

import (
	"fmt"
	"math"
	"time"
)

type series struct {
	start, last, coverageStart time.Time
	low, high                  float64
	count, coverageCount       int
}

func number(v float64) string { return fmt.Sprintf("%.3f", v) }

func (e *evaluator) temporal(p map[string]point, fresh map[string]bool) {
	for _, rule := range []struct {
		key, code, symptom string
		threshold          float64
		max                time.Duration
		drop               bool
	}{
		{"iat_c", "iat_jump", "IAT изменилась не менее чем на 15 °C за 10 секунд.", 15, 10 * time.Second, false},
		{"coolant_c", "coolant_jump", "ECT изменилась не менее чем на 15 °C за 10 секунд.", 15, 10 * time.Second, false},
		{"voltage_v", "adapter_voltage_drop", "Падение питания адаптера не менее 1.5 V; напряжение на клеммах не измерено.", 1.5, 130 * time.Second, true},
	} {
		v, ok := p[rule.key]
		previous, had := e.last[rule.key]
		if !ok || !fresh[rule.key] || !had {
			continue
		}
		dt := v.at.Sub(previous.at)
		if dt <= 0 || dt > rule.max {
			continue
		}
		e.checks[rule.code] = true
		delta := math.Abs(v.value - previous.value)
		if rule.drop {
			delta = previous.value - v.value
		}
		if delta >= rule.threshold {
			e.flag(rule.code, rule.symptom, "observed", v.evidence)
			e.previousEvidence(rule.code, previous.evidence)
		}
	}
	rpm, running := p["rpm"]
	running = running && rpm.value > 0
	e.constant(p, fresh, "iat_c", "iat_constant", running, 20*time.Second, 5*time.Minute, .5)
	ect, warm := p["coolant_c"]
	warm = warm && ect.value >= 70 && running
	for _, k := range oxygenKeys {
		e.constant(p, fresh, k, "o2_constant", warm, 5*time.Second, 30*time.Second, .02)
	}
}

func (e *evaluator) previousEvidence(code string, ev Evidence) {
	for n := range e.findings {
		f := &e.findings[n]
		if f.Code == code && len(f.Supporting) < maxExamples {
			ev.Detail = "Предыдущее независимое измерение для сравнения."
			f.Supporting = append(f.Supporting, ev)
			return
		}
	}
}

func (e *evaluator) constant(p map[string]point, fresh map[string]bool, key, code string, enabled bool, maxStep, minSpan time.Duration, maxRange float64) {
	v, ok := p[key]
	if !ok || !enabled {
		delete(e.series, key)
		return
	}
	if !fresh[key] {
		return
	}
	s, had := e.series[key]
	if !had || v.at.Sub(s.last) > maxStep || v.at.Before(s.last) {
		s = series{start: v.at, coverageStart: v.at, low: v.value, high: v.value}
	}
	s.last = v.at
	s.count++
	s.coverageCount++
	s.low = math.Min(s.low, v.value)
	s.high = math.Max(s.high, v.value)
	if s.coverageCount >= 10 && s.last.Sub(s.coverageStart) >= minSpan {
		e.adequate[code] = true
	}
	if s.high-s.low > maxRange {
		s.start = v.at
		s.low = v.value
		s.high = v.value
		s.count = 1
	}
	e.series[key] = s
	if s.count >= 10 && s.last.Sub(s.start) >= minSpan {
		e.flag(code, "Длительно постоянное значение "+key+"; само по себе не доказывает неисправность.", "limited_hypothesis", v.evidence)
	}
}
