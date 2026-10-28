package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuelhistory"
)

func historyOptions(r *http.Request) (fuelhistory.Options, error) {
	o := fuelhistory.Options{}
	if len(r.URL.RawQuery) > 2048 {
		return o, fmt.Errorf("history query too long")
	}
	q := r.URL.Query()
	for key, values := range q {
		if len(values) != 1 {
			return o, fmt.Errorf("duplicate history query")
		}
		switch key {
		case "session", "from", "to", "points":
		default:
			return o, fmt.Errorf("unknown history query field")
		}
	}
	o.Session = q.Get("session")
	for _, item := range []struct {
		value  string
		target *time.Time
	}{{q.Get("from"), &o.From}, {q.Get("to"), &o.To}} {
		if item.value != "" {
			value, err := time.Parse(time.RFC3339Nano, item.value)
			if err != nil {
				return o, err
			}
			*item.target = value
		}
	}
	if q.Get("points") != "" {
		n, err := strconv.Atoi(q.Get("points"))
		if err != nil || n <= 0 {
			return o, fmt.Errorf("invalid point count")
		}
		o.MaxPoints = n
	}
	if !o.From.IsZero() && !o.To.IsZero() && o.To.Sub(o.From) > 24*time.Hour {
		return o, fmt.Errorf("history selection must be at most 24 hours")
	}
	return o, o.Validate()
}
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if s.config.HistoryDir == "" {
		http.Error(w, "history is not configured", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/api/fuel/history/sessions" {
		if r.URL.RawQuery != "" {
			http.Error(w, "sessions endpoint takes no query", 400)
			return
		}
		sessions, err := fuelhistory.List(s.config.HistoryDir)
		if err != nil {
			http.Error(w, "cannot read history catalog", 409)
			return
		}
		writeHistory(w, sessions)
		return
	}
	options, err := historyOptions(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	report, err := fuelhistory.Analyze(ctx, s.config.HistoryDir, options)
	if err != nil {
		http.Error(w, "cannot read selected history; catalog may have rotated: "+err.Error(), 409)
		return
	}
	writeHistory(w, report)
}
func writeHistory(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "cannot encode history", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(data); err != nil {
		return
	} // The reader disconnected.
}
