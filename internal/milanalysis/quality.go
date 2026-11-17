package milanalysis

import (
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func (e *evaluator) quality(f frameRecord) {
	if f.frame.EffectiveHealth != telemetry.Connected {
		e.gaps = true
		e.reset()
		e.flag("source_unavailable", "Источник телеметрии неприменим на время кадра.", "observed", Evidence{Reference: f.ref, Detail: string(f.frame.EffectiveHealth)})
	}
	if e.previous != nil {
		before, after := e.previous.frame.Snapshot.Diagnostics, f.frame.Snapshot.Diagnostics
		for _, v := range []struct {
			code          string
			before, after uint64
		}{{"counter_errors", before.Errors, after.Errors}, {"counter_gaps", before.Gaps, after.Gaps}, {"counter_recoveries", before.Recoveries, after.Recoveries}} {
			if v.after > v.before {
				e.flag(v.code, "Сохранён рост счётчика качества телеметрии; причина не установлена.", "observed", Evidence{Reference: f.ref, Detail: fmt.Sprintf("%d -> %d", v.before, v.after)})
			}
		}
	}
	p := f.frame.Snapshot.Poll
	if p == nil {
		return
	}
	key := fmt.Sprintf("%d/%s/%s", p.Generation, p.Command, p.At.Format(time.RFC3339Nano))
	if e.polls[key] {
		return
	}
	e.polls[key] = true
	ev := Evidence{Reference: f.ref, CapturedAt: p.At, Detail: excerpt(p.Command + ": " + p.Status + " " + p.Error)}
	if p.LatencyNS >= int64(time.Second) {
		v := float64(p.LatencyNS) / float64(time.Millisecond)
		ev.Value = &v
		ev.Unit = "ms"
		e.flag("poll_delay", "Опрос занял не менее секунды; CPU/BLE/ЭБУ по этому факту не различаются.", "observed", ev)
	}
	if p.Error != "" || p.Status == "invalid_response" || p.Status == "unavailable" || p.Status == "temporarily_unavailable" {
		e.flag("protocol_error", "Опрос завершился ошибкой или временной недоступностью.", "observed", ev)
	}
}
