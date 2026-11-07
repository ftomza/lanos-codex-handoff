package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

const milHistoryResponseLimit = 8 << 20
const milHistoryTimeout = 8 * time.Second

var errMILHistorySize = errors.New("MIL history response exceeds limit")

func milHistoryOptions(r *http.Request, export bool) (milanalysis.Options, error) {
	o := milanalysis.Options{}
	if len(r.URL.RawQuery) > 2048 {
		return o, fmt.Errorf("MIL history query too long")
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return o, fmt.Errorf("invalid MIL history query")
	}
	for key, values := range q {
		if key != "session" && key != "incident" || len(values) != 1 || values[0] == "" {
			return o, fmt.Errorf("invalid MIL history selection")
		}
	}
	o.Session, o.Incident = q.Get("session"), q.Get("incident")
	if o.Session == "" || export && o.Incident == "" {
		return o, fmt.Errorf("MIL history selection is required")
	}
	return o, o.Validate()
}

func (s *Server) handleMILHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.config.MILHistoryDir == "" {
		http.Error(w, "MIL history is not configured", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), milHistoryTimeout)
	defer cancel()
	if r.URL.Path == "/api/mil/history/sessions" {
		if r.URL.RawQuery != "" {
			http.Error(w, "MIL sessions endpoint takes no query", http.StatusBadRequest)
			return
		}
		catalog, err := milstore.List(ctx, s.config.MILHistoryDir)
		if err != nil {
			milHistoryError(w, ctx, err)
			return
		}
		writeMILHistory(w, ctx, catalog)
		return
	}
	export := r.URL.Path == "/api/mil/history/incident"
	options, err := milHistoryOptions(r, export)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := ctx.Err(); err != nil {
		milHistoryError(w, ctx, err)
		return
	}
	// Heavy readers never hold the telemetry mutex and never queue.
	select {
	case s.milHistoryGate <- struct{}{}:
		defer func() { <-s.milHistoryGate }()
	default:
		http.Error(w, "MIL history analysis is busy; retry later", http.StatusTooManyRequests)
		return
	}
	var result any
	if export {
		result, err = readMILIncident(ctx, s.config.MILHistoryDir, options)
	} else {
		result, err = milanalysis.Analyze(ctx, s.config.MILHistoryDir, options)
	}
	if err != nil {
		milHistoryError(w, ctx, err)
		return
	}
	writeMILHistory(w, ctx, result)
}

func milHistoryError(w http.ResponseWriter, ctx context.Context, err error) {
	switch {
	case ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "MIL history request expired or was canceled", http.StatusGatewayTimeout)
	case errors.Is(err, errMILHistorySize):
		http.Error(w, "MIL history response exceeds 8 MiB", http.StatusRequestEntityTooLarge)
	default:
		http.Error(w, "cannot read selected MIL history; refresh the catalog and retry", http.StatusConflict)
	}
}

type milHistoryBuffer struct{ bytes.Buffer }

func (b *milHistoryBuffer) Write(p []byte) (int, error) {
	if len(p) > milHistoryResponseLimit-b.Len() {
		return 0, errMILHistorySize
	}
	return b.Buffer.Write(p)
}

// Buffer before committing headers so an oversized/invalid result never becomes
// a successful partial JSON download.
func writeMILHistory(w http.ResponseWriter, ctx context.Context, result any) {
	var b milHistoryBuffer
	if err := json.NewEncoder(&b).Encode(result); err != nil {
		if errors.Is(err, errMILHistorySize) {
			milHistoryError(w, ctx, err)
		} else {
			http.Error(w, "cannot encode MIL history", http.StatusInternalServerError)
		}
		return
	}
	if err := ctx.Err(); err != nil {
		milHistoryError(w, ctx, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b.Bytes())
}
