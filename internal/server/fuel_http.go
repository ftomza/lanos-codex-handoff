package server

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
)

type fuelRequest struct {
	PreviewToken  string  `json:"preview_token"`
	Revision      uint64  `json:"expected_revision"`
	Liters        float64 `json:"measured_liters"`
	FullToFull    bool    `json:"full_to_full"`
	DefaultFactor bool    `json:"default_factor"`
	Action        string  `json:"action"`
}

func sameOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return err == nil && origin.Scheme == scheme && origin.Host == r.Host && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}
func readFuelRequest(w http.ResponseWriter, r *http.Request) (fuelRequest, bool) {
	var req fuelRequest
	if !sameOrigin(r) {
		http.Error(w, "same-origin request required", http.StatusForbidden)
		return req, false
	}
	content, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || content != "application/json" {
		http.Error(w, "application/json required", http.StatusUnsupportedMediaType)
		return req, false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid fuel request", http.StatusBadRequest)
		return req, false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		http.Error(w, "one JSON object required", http.StatusBadRequest)
		return req, false
	}
	return req, true
}
func (s *Server) handleFuel(w http.ResponseWriter, r *http.Request) {
	if s.config.Fuel == nil {
		http.Error(w, "fuel computer disabled", http.StatusServiceUnavailable)
		return
	}
	req, ok := readFuelRequest(w, r)
	if !ok {
		return
	}
	var result any
	var err error
	switch r.URL.Path {
	case "/api/fuel/reset":
		err = s.config.Fuel.Reset(req.Revision, req.DefaultFactor, time.Now())
	case "/api/fuel/calibration/preview":
		result, err = s.config.Fuel.Preview(req.Liters, req.FullToFull, req.Revision)
	case "/api/fuel/calibration":
		err = s.config.Fuel.Apply(req.Action, req.Liters, req.FullToFull, req.Revision, req.PreviewToken, time.Now())
	}
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, fuel.ErrRevision) {
			code = http.StatusConflict
		}
		if errors.Is(err, fuel.ErrCalibration) {
			code = http.StatusBadRequest
		}
		http.Error(w, err.Error(), code)
		return
	}
	if result == nil {
		result = s.config.Fuel.View(time.Now())
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	// Values are validated by the domain; encode before writing response headers.
	data, err := json.Marshal(result)
	if err != nil {
		http.Error(w, "cannot encode fuel result", http.StatusInternalServerError)
		return
	}
	if _, err := w.Write(data); err != nil {
		return
	}
}
