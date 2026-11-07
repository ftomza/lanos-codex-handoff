package milanalysis

import (
	"slices"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type diagnosticPayload struct {
	Command string               `json:"command"`
	Reading telemetry.ECUReading `json:"reading"`
	Poll    telemetry.Poll       `json:"poll"`
}

func (a *analyzer) diagnostic(index int, e milstore.ReadEntry) error {
	var v diagnosticPayload
	if err := payload(e.Data, &v); err != nil {
		return err
	}
	detail, err := validateDiagnostic(v, e.At)
	if err != nil {
		return err
	}
	s := &a.report.Sessions[index]
	for _, d := range s.Diagnostics {
		if d.Command == v.Command && d.Generation == v.Poll.Generation && d.Reading.CapturedAt.Equal(v.Reading.CapturedAt) {
			return nil
		}
	}
	if len(s.Diagnostics) >= maxItems {
		s.Incomplete = true
		a.issue(milstore.ReadIssue{Code: "diagnostics_truncated", Detail: "diagnostic limit reached"})
		return nil
	}
	r := reference(e)
	r.Generation = v.Poll.Generation
	d := Diagnostic{Reference: r, Command: v.Command, Poll: v.Poll, Reading: Reading{Status: excerpt(v.Reading.Status), CapturedAt: v.Reading.CapturedAt, ReceivedAt: v.Reading.ReceivedAt, RawResponse: excerpt(v.Reading.RawResponse), Detail: detail}}
	d.Poll.Error = excerpt(d.Poll.Error)
	s.Diagnostics = append(s.Diagnostics, d)
	if v.Reading.Status == "available" && slices.Contains(detail.Codes, "P0420") {
		findings := &s.Findings
		if e.Incident != "" {
			for n := range s.Incidents {
				if s.Incidents[n].ID == e.Incident {
					findings = &s.Incidents[n].Findings
					break
				}
			}
		}
		addFinding(findings, "p0420", "Сохранён P0420: эффективность каталитической системы ниже порога, банк 1.", "observed", Evidence{Reference: r, CapturedAt: v.Reading.CapturedAt, Detail: v.Command + ": " + excerpt(v.Reading.RawResponse)}, []string{"Синхронные достаточно частые измерения обоих O2.", "Проверка герметичности выпуска, смеси и цепей O2; код не определяет неисправную деталь."}, []string{"Проверить выпуск, смесь/зажигание и оба O2 перед выводом о катализаторе. Сохранить коды и freeze frame."})
	}
	return nil
}

func diagnosticCommand(c string) bool {
	if c == "03" || c == "07" || c == "0A" {
		return true
	}
	if len(c) != 6 || c[:2] != "02" {
		return false
	}
	for _, b := range c {
		if !(b >= '0' && b <= '9' || b >= 'A' && b <= 'F') {
			return false
		}
	}
	return true
}

func addFinding(findings *[]Finding, code, symptom, confidence string, e Evidence, missing, next []string) {
	for n := range *findings {
		f := &(*findings)[n]
		if f.Code != code {
			continue
		}
		f.Count++
		if e.At.Before(f.FirstObservedAt) {
			f.FirstObservedAt = e.At
		}
		if e.At.After(f.LastObservedAt) {
			f.LastObservedAt = e.At
		}
		if len(f.Supporting) < maxExamples {
			f.Supporting = append(f.Supporting, e)
		}
		return
	}
	if len(*findings) >= maxFindings {
		return
	}
	*findings = append(*findings, Finding{Code: code, Symptom: symptom, Confidence: confidence, Count: 1, FirstObservedAt: e.At, LastObservedAt: e.At, Supporting: []Evidence{e}, Contradicting: []Evidence{}, Missing: missing, NextCheck: next})
}
