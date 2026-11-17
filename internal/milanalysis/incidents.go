package milanalysis

import (
	"cmp"
	"fmt"
	"slices"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

type frameRecord struct {
	frame   mil.Frame
	ref     Reference
	segment int
}
type incidentState struct {
	session, index int
	frames         []frameRecord
	broken         bool
	ref            Reference
	limited        bool
}

func (a *analyzer) window(index int, e milstore.ReadEntry) error {
	var w mil.Window
	if err := payload(e.Data, &w); err != nil {
		return err
	}
	if w.ID == "" || w.ID != e.Incident || w.TriggeredAt.IsZero() || w.PostUntil.Before(w.TriggeredAt) {
		return fmt.Errorf("invalid incident window")
	}
	if e.Kind == "incident_start" {
		a.finishActive()
		s := &a.report.Sessions[index]
		if len(s.Incidents) >= maxItems {
			a.issue(milstore.ReadIssue{Code: "incidents_truncated", Detail: "incident limit reached"})
			s.Incomplete = true
			return nil
		}
		for _, i := range s.Incidents {
			if i.ID == w.ID {
				return fmt.Errorf("duplicate incident id")
			}
		}
		s.Incidents = append(s.Incidents, Incident{ID: w.ID, Window: w, Findings: []Finding{}, Checks: []RuleCheck{}})
		a.active = &incidentState{session: index, index: len(s.Incidents) - 1, frames: make([]frameRecord, 0, 128), ref: reference(e)}
		return nil
	}
	if a.active == nil || a.active.session != index || a.report.Sessions[index].Incidents[a.active.index].ID != w.ID {
		return fmt.Errorf("incident continuation without matching start")
	}
	i := &a.report.Sessions[index].Incidents[a.active.index]
	i.Window = w
	if e.Kind == "incident_end" {
		i.Closed = true
		a.finishActive()
	}
	return nil
}

func (a *analyzer) frame(index int, e milstore.ReadEntry) error {
	if a.active == nil || a.active.session != index || a.report.Sessions[index].Incidents[a.active.index].ID != e.Incident {
		return fmt.Errorf("frame without matching incident")
	}
	var f mil.Frame
	if err := payload(e.Data, &f); err != nil {
		return err
	}
	if f.At.IsZero() || !f.At.Equal(e.At) || f.Snapshot.Source != e.Source {
		return fmt.Errorf("invalid context frame")
	}
	if len(a.active.frames) >= maxFrames {
		if !a.active.limited {
			a.issue(milstore.ReadIssue{Code: "frames_truncated", File: e.File, Line: e.Line, Detail: "incident frame limit reached"})
		}
		a.active.limited = true
		a.active.broken = true
		return nil
	}
	r := reference(e)
	r.Generation = f.Snapshot.Generation
	a.active.frames = append(a.active.frames, frameRecord{frame: f, ref: r, segment: a.segment})
	return nil
}

func (a *analyzer) finishActive() {
	if a.active == nil {
		return
	}
	s := a.active
	a.active = nil
	i := &a.report.Sessions[s.session].Incidents[s.index]
	slices.SortStableFunc(s.frames, func(a, b frameRecord) int {
		if c := a.frame.At.Compare(b.frame.At); c != 0 {
			return c
		}
		return cmp.Compare(a.ref.Sequence, b.ref.Sequence)
	})
	unique := make([]frameRecord, 0, len(s.frames))
	for _, f := range s.frames {
		if len(unique) > 0 {
			p := unique[len(unique)-1]
			if p.frame.At.Equal(f.frame.At) && p.ref.Generation == f.ref.Generation {
				continue
			}
		}
		unique = append(unique, f)
	}
	i.FrameCount = len(unique)
	i.Incomplete = !i.Closed || !i.Window.PreWindowRetained || !i.Window.PostWindowRetained || i.Window.HadGaps || s.broken || len(unique) == 0
	if len(unique) > 0 && (unique[0].frame.At.After(i.Window.RequestedPreFrom) || unique[len(unique)-1].frame.At.Before(i.Window.PostUntil)) {
		i.Incomplete = true
	}
	findings, checks, gaps := evaluate(unique)
	i.Findings = append(i.Findings, findings...)
	i.Checks = checks
	i.Incomplete = i.Incomplete || gaps
	if i.Incomplete {
		addFinding(&i.Findings, "incomplete_context", "Контекст события сохранён не полностью или содержит пропуски.", "observed", Evidence{Reference: s.ref, Detail: "Проверяйте исходные retained/had_gaps/reason и фактические кадры."}, []string{"Непрерывная телеметрия полного окна."}, []string{"Повторить запись на исправно работающем хосте, сохранив прежний архив."})
	}
}
