package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/web"
)

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/fuel/reset", s.handleFuel)
	mux.HandleFunc("POST /api/fuel/calibration/preview", s.handleFuel)
	mux.HandleFunc("POST /api/fuel/calibration", s.handleFuel)
	mux.HandleFunc("GET /api/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.Handle("GET /", http.FileServerFS(web.Assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	// Encoding into a buffer detects failure before sending a successful status.
	data, err := json.Marshal(s.snapshot())
	if err != nil {
		http.Error(w, "cannot encode telemetry", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(data); err != nil {
		return // The client has gone away; no further response can be delivered.
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(s.config.StreamInterval)
	defer ticker.Stop()
	for {
		if err := s.writeEvent(w); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) writeEvent(w http.ResponseWriter) error {
	controller := http.NewResponseController(w)
	if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return err
	}
	data, err := json.Marshal(s.snapshot())
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", data); err != nil {
		return err
	}
	return controller.Flush()
}
