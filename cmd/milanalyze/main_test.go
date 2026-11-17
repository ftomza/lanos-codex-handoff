package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

func TestOfflineHelpAndValidation(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(t.Context(), []string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "input") {
		t.Fatal(err, out.String())
	}
	for _, args := range [][]string{{}, {"--input=x", "extra"}, {"--input=x", "--session=../bad"}, {"--input=x", "--incident=incident-0000000000000001"}, {"--input=missing"}} {
		if err := run(t.Context(), args, io.Discard); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestInvalidFlagDoesNotContaminateReportOutput(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(t.Context(), []string{"--unknown-option"}, &out); err == nil {
		t.Fatal("accepted unknown flag")
	}
	if out.Len() != 0 {
		t.Fatal("flag diagnostics leaked into report", out.String())
	}
}

func TestEmptyClosedArchiveTextJSONAndCancellation(t *testing.T) {
	t.Parallel()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := milstore.Open(milstore.DefaultConfig(dir, "live"))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(t.Context(), []string{"--input=" + dir}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "UTC+03:00") || !strings.Contains(out.String(), "не доказывает") || !strings.Contains(out.String(), "нет сохранённого запроса") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := run(t.Context(), []string{"--input=" + dir, "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"version":1`) {
		t.Fatal(out.String())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := run(ctx, []string{"--input=" + dir}, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"--input=" + dir}, brokenWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if err := run(t.Context(), []string{"--input=" + dir, "--json"}, brokenWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTextKeepsUnknownAndDiagnosticAbsenceDistinct(t *testing.T) {
	t.Parallel()
	r := milanalysis.Report{Version: 1, Source: "live", Incomplete: true, Sessions: []milanalysis.Session{{ID: "s", StartedAt: time.Date(2026, 11, 7, 13, 54, 18, 0, time.UTC), Summary: "MIL UNKNOWN"}}, Limitations: []string{"Нет подтверждённого OFF."}}
	var out bytes.Buffer
	if err := text(&out, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "16:54:18") || !strings.Contains(out.String(), "MIL UNKNOWN") || !strings.Contains(out.String(), "нет сохранённого запроса") {
		t.Fatal(out.String())
	}
}
