package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
)

var moscow = time.FixedZone("Europe/Moscow", 3*60*60)

type printer struct {
	out io.Writer
	err error
}

func (p *printer) print(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.out, format, args...)
	}
}
func date(t time.Time) string {
	if t.IsZero() {
		return "нет данных"
	}
	return t.In(moscow).Format(time.RFC3339Nano)
}

func text(out io.Writer, r milanalysis.Report) error {
	p := &printer{out: out}
	p.print("MIL-анализ v%d — %s; время Europe/Moscow (UTC+03:00)\nНеполные данные: %t\n", r.Version, r.Source, r.Incomplete)
	for _, s := range r.Sessions {
		sessionText(p, s)
	}
	for _, issue := range r.Issues {
		p.print("Проблема архива: %s %s:%d %s\n", issue.Code, issue.File, issue.Line, issue.Detail)
	}
	for _, l := range r.Limitations {
		p.print("Ограничение: %s\n", l)
	}
	return p.err
}

func sessionText(p *printer, s milanalysis.Session) {
	p.print("\nСеанс %s: %s — %s; закрыт: %t; неполный: %t\n%s\n", s.ID, date(s.StartedAt), date(s.EndedAt), s.Closed, s.Incomplete, s.Summary)
	if s.EndReason != "" || s.Error != "" {
		p.print("Завершение: %s %s\n", s.EndReason, s.Error)
	}
	for _, e := range s.Events {
		p.print("MIL %s: %s (%s), generation=%d [%s:%d]\n", date(e.Event.At), e.Event.Kind, e.Event.State, e.Generation, e.File, e.Line)
		if !e.Event.PreviousCapturedAt.IsZero() {
			p.print("  Предыдущее подтверждённое наблюдение: %s; точное время изменения ЭБУ неизвестно.\n", date(e.Event.PreviousCapturedAt))
		}
	}
	seen := make(map[string]bool, 3)
	for _, d := range s.Diagnostics {
		seen[d.Command] = true
		association := d.Incident
		if association == "" {
			association = "отдельный ответ без привязки к событию"
		}
		p.print("Диагностика %s %s: %s; %s [%s:%d]\n", date(d.Reading.CapturedAt), d.Command, d.Reading.Status, association, d.File, d.Line)
		if d.Reading.Status == "available" && (d.Command == "03" || d.Command == "07" || d.Command == "0A" || d.Reading.Detail.PID == "02") {
			p.print("  Коды: [%s]\n", strings.Join(d.Reading.Detail.Codes, ", "))
		}
		if v := d.Reading.Detail.Value; v != nil {
			p.print("  Freeze frame PID %s: %.3f %s (сохранённый контекст ЭБУ)\n", d.Reading.Detail.PID, *v, d.Reading.Detail.Unit)
		}
		if len(d.Reading.Detail.Supported) > 0 {
			p.print("  Поддерживаемые PID кадра: %s\n", strings.Join(d.Reading.Detail.Supported, ", "))
		}
		p.print("  Raw: %q\n", d.Reading.RawResponse)
	}
	for _, c := range []string{"03", "07", "0A"} {
		if !seen[c] {
			p.print("Диагностика %s: нет сохранённого запроса; это не пустой список кодов.\n", c)
		}
	}
	for _, f := range s.Findings {
		findingText(p, f)
	}
	for _, i := range s.Incidents {
		p.print("\nСобытие %s: %s; кадров: %d; закрыто: %t; неполное: %t\n  pre_retained=%t post_retained=%t had_gaps=%t reason=%s\n", i.ID, date(i.Window.TriggeredAt), i.FrameCount, i.Closed, i.Incomplete, i.Window.PreWindowRetained, i.Window.PostWindowRetained, i.Window.HadGaps, i.Window.Reason)
		for _, f := range i.Findings {
			findingText(p, f)
		}
		for _, c := range i.Checks {
			if c.Status == "insufficient_data" {
				p.print("  %s: недостаточно данных — %s\n", c.Code, c.Detail)
			}
		}
	}
}

func findingText(p *printer, f milanalysis.Finding) {
	p.print("  %s [%s], наблюдений %d: %s\n", f.Code, f.Confidence, f.Count, f.Symptom)
	p.print("    Границы наблюдений: %s — %s\n", date(f.FirstObservedAt), date(f.LastObservedAt))
	for _, e := range f.Supporting {
		p.print("    Факт: %s %s", date(e.At), e.Signal)
		if e.Value != nil {
			p.print("=%.3f %s", *e.Value, e.Unit)
		}
		p.print("; capture=%s; %s [%s:%d, seq=%d]\n", date(e.CapturedAt), e.Detail, e.File, e.Line, e.Sequence)
	}
	if len(f.Contradicting) == 0 {
		p.print("    Противоречия: не найдены в доступных данных; это не доказывает их отсутствия.\n")
	}
	for _, e := range f.Contradicting {
		p.print("    Другие наблюдения: %s %s; %s [%s:%d]\n", date(e.CapturedAt), e.Signal, e.Detail, e.File, e.Line)
	}
	for _, m := range f.Missing {
		p.print("    Не хватает: %s\n", m)
	}
	for _, n := range f.NextCheck {
		p.print("    Следующая проверка: %s\n", n)
	}
}
