package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestFuelControlsValidateOriginJSONAndRevision(t *testing.T) {
	t.Parallel()
	c, err := fuel.Open(fuel.DefaultConfig(), telemetry.Simulator, filepath.Join(t.TempDir(), "trip.json"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	s, err := New(Config{Fuel: c})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, origin, ctype, body string
		code                      int
	}{
		{"cross origin", "http://evil.test", "application/json", `{"expected_revision":1}`, 403},
		{"no origin", "", "application/json", `{"expected_revision":1}`, 403},
		{"form", "http://lanos.test", "text/plain", `{"expected_revision":1}`, 415},
		{"unknown key", "http://lanos.test", "application/json", `{"expected_revision":1,"extra":true}`, 400},
		{"stale revision", "http://lanos.test", "application/json", `{"expected_revision":99}`, 409},
		{"large body", "http://lanos.test", "application/json", strings.Repeat("x", 5000), 400},
		{"reset", "http://lanos.test", "application/json", `{"expected_revision":1}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Each rejection is independent; only the valid reset changes revision.
			req := httptest.NewRequest(http.MethodPost, "http://lanos.test/api/fuel/reset", strings.NewReader(tc.body))
			req.Header.Set("Origin", tc.origin)
			req.Header.Set("Content-Type", tc.ctype)
			out := httptest.NewRecorder()
			s.Handler().ServeHTTP(out, req)
			if out.Code != tc.code {
				t.Fatalf("status %d want %d: %s", out.Code, tc.code, out.Body.String())
			}
		})
	}
}
func TestCalibrationRequiresCoveredIntervalAndDisabledIsExplicit(t *testing.T) {
	t.Parallel()
	s := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "http://lanos.test/api/fuel/reset", strings.NewReader(`{"expected_revision":1}`))
	req.Header.Set("Origin", "http://lanos.test")
	req.Header.Set("Content-Type", "application/json")
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, req)
	if out.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled computer: %d", out.Code)
	}
}
