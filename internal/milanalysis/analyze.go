package milanalysis

import (
	"context"
	"fmt"
	"slices"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

type analyzer struct {
	report  Report
	options Options
	indexes map[string]int
	active  *incidentState
	segment int
}

func Analyze(ctx context.Context, dir string, options Options) (Report, error) {
	if err := options.Validate(); err != nil {
		return Report{}, err
	}
	a := analyzer{options: options, indexes: make(map[string]int), report: Report{Version: 1, Sessions: []Session{}, Issues: []milstore.ReadIssue{}, Limitations: []string{
		"Анализируются только сохранённые наблюдения; отсутствие находок не доказывает исправность автомобиля.",
		"MIL-архив содержит контекст событий, а не полную поездку. Пропуски не восстанавливаются.",
		"Гипотезы и пороги эвристик не устанавливают неисправную деталь; DTC не стираются.",
		"Задержки не определяют причину CPU/BLE/ЭБУ; несинхронные O2 не доказывают эффективность катализатора.",
	}}}
	issues, err := milstore.Read(ctx, dir, milstore.ReadOptions{Session: options.Session}, a.consume)
	if err != nil {
		return Report{}, err
	}
	a.finishActive()
	for _, i := range issues {
		a.issue(i)
	}
	if len(a.report.Sessions) == 0 {
		return Report{}, fmt.Errorf("no saved MIL sessions found")
	}
	found := options.Incident == ""
	for n := range a.report.Sessions {
		s := &a.report.Sessions[n]
		if !s.Closed {
			s.Incomplete = true
			a.report.Incomplete = true
		}
		for _, i := range s.Incidents {
			if i.ID == options.Incident {
				found = true
			}
			if i.Incomplete {
				s.Incomplete = true
				a.report.Incomplete = true
			}
		}
		if options.Incident != "" {
			s.Incidents = slices.DeleteFunc(s.Incidents, func(i Incident) bool { return i.ID != options.Incident })
		}
		s.Summary = sessionSummary(*s)
	}
	if !found {
		return Report{}, fmt.Errorf("saved MIL incident not found")
	}
	return a.report, nil
}

func (a *analyzer) issue(i milstore.ReadIssue) {
	a.report.Incomplete = true
	if len(a.report.Issues) < 127 {
		a.report.Issues = append(a.report.Issues, i)
		return
	}
	if len(a.report.Issues) == 127 {
		a.report.Issues = append(a.report.Issues, milstore.ReadIssue{Code: "issues_truncated", Detail: "additional issues omitted"})
	}
}

func (a *analyzer) consume(e milstore.ReadEntry) error {
	if e.Boundary {
		a.segment++
		if a.active != nil {
			a.active.broken = true
		}
	}
	index, ok := a.indexes[e.Session]
	if !ok {
		if len(a.report.Sessions) >= maxSessions {
			a.issue(milstore.ReadIssue{Code: "sessions_truncated", Detail: "session limit reached"})
			return nil
		}
		index = len(a.report.Sessions)
		a.indexes[e.Session] = index
		a.report.Sessions = append(a.report.Sessions, Session{ID: e.Session, Source: e.Source, StartedAt: e.At, Events: []Event{}, Diagnostics: []Diagnostic{}, Incidents: []Incident{}, Findings: []Finding{}})
	}
	if a.active != nil && a.active.session != index {
		a.finishActive()
	}
	a.report.Source = e.Source
	s := &a.report.Sessions[index]
	var err error
	switch e.Kind {
	case "session_start":
		s.StartedAt = e.At
	case "session_end":
		err = a.endSession(index, e)
	case "event":
		err = a.event(index, e)
	case "diagnostic":
		err = a.diagnostic(index, e)
	case "incident_start", "incident_extend", "incident_end":
		err = a.window(index, e)
	case "pre", "post":
		err = a.frame(index, e)
	}
	if err != nil {
		a.issue(milstore.ReadIssue{Code: "invalid_payload", File: e.File, Line: e.Line, Detail: excerpt(err.Error())})
		s.Incomplete = true
		a.segment++
		if a.active != nil {
			a.active.broken = true
		}
	}
	return nil
}

func (a *analyzer) endSession(index int, e milstore.ReadEntry) error {
	var footer struct {
		Reason string `json:"reason"`
		Error  string `json:"error"`
	}
	if err := payload(e.Data, &footer); err != nil {
		return err
	}
	s := &a.report.Sessions[index]
	s.Closed = true
	s.EndedAt = e.At
	s.EndReason = excerpt(footer.Reason)
	s.Error = excerpt(footer.Error)
	if footer.Reason != "closed" || footer.Error != "" {
		s.Incomplete = true
		a.issue(milstore.ReadIssue{Code: "recording_failed", File: e.File, Line: e.Line, Detail: excerpt(footer.Reason + ": " + footer.Error)})
	}
	a.finishActive()
	return nil
}

func (a *analyzer) event(index int, e milstore.ReadEntry) error {
	var v mil.Event
	if err := payload(e.Data, &v); err != nil {
		return err
	}
	if v.At.IsZero() || v.ID == 0 || !slices.Contains([]string{"MIL_ON", "MIL_OFF", "MIL_INITIAL_OFF", "MIL_ALREADY_ON", "MIL_ON_AFTER_GAP", "MIL_OFF_AFTER_GAP", "MIL_UNKNOWN"}, v.Kind) {
		return fmt.Errorf("invalid MIL event")
	}
	s := &a.report.Sessions[index]
	for _, p := range s.Events {
		if p.Event.ID == v.ID {
			return fmt.Errorf("duplicate MIL event id")
		}
	}
	if len(s.Events) >= maxItems {
		s.Incomplete = true
		a.issue(milstore.ReadIssue{Code: "events_truncated", Detail: "event limit reached"})
		return nil
	}
	if v.Observation != nil {
		v.Observation.RawResponse = excerpt(v.Observation.RawResponse)
		if len(v.Observation.Readiness) > 4096 {
			v.Observation.Readiness = nil
			s.Incomplete = true
		}
	}
	r := reference(e)
	r.Generation = v.Generation
	s.Events = append(s.Events, Event{Reference: r, Event: v})
	return nil
}

func sessionSummary(s Session) string {
	for _, d := range s.Diagnostics {
		if d.Reading.Status == "available" && len(d.Reading.Detail.Codes) > 0 {
			return "В записи сохранены диагностические коды; причина требует проверки."
		}
	}
	for _, e := range s.Events {
		if e.Event.State == "ON" {
			return "Check Engine включён. Причина не определена стандартными диагностическими режимами"
		}
	}
	return "Подтверждённого включения MIL в доступной истории нет; это не заключение об исправности автомобиля."
}
