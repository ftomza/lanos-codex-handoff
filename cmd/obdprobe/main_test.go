package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestNoImplicitHardwareAccess(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{}, {"--baud=123"}, {"--port=/dev/cu.fake", "--timeout=0s"}, {"--port=/dev/cu.fake", "--retries=6"}, {"--help"}, {"--list-ports", "extra"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			calls := 0
			deps := dependencies{listPorts: func() ([]string, error) { return []string{}, nil }, probe: func(context.Context, probe.Config) (probe.Report, error) { calls++; return probe.Report{}, nil }}
			err := run(t.Context(), args, &out, &stderr, deps)
			if len(args) == 1 && args[0] == "--help" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatalf("accepted %v", args)
			}
			if calls != 0 {
				t.Fatal("implicit hardware access")
			}
		})
	}
}

func TestEmptyPortListGuidance(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	err := run(t.Context(), []string{"--list-ports", "--json"}, &out, &stderr, dependencies{listPorts: func() ([]string, error) { return []string{}, nil }})
	if err != nil || !strings.Contains(out.String(), "SPP") || !json.Valid(out.Bytes()) {
		t.Fatalf("guidance: %v %s", err, out.String())
	}
}

func TestReportOutputAndVINRedaction(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "probe.json")
	var out, stderr bytes.Buffer
	calls := 0
	const vin = "1HGCM82633A004352"
	deps := dependencies{probe: func(ctx context.Context, cfg probe.Config) (probe.Report, error) {
		calls++
		if cfg.Port != "/dev/cu.fake" || cfg.Baud != 9600 || !cfg.ScanSupported || cfg.InitialTimeout != 30*time.Second || cfg.AdapterModel != "Vgate iCar Pro BLE 4.0 Dual" {
			t.Fatalf("flags: %+v", cfg)
		}
		return probe.Report{Source: "test_fixture", Status: "partial", Commands: []probe.CommandLog{{RawResponse: "unsolicited VIN: " + vin + "\r>"}}}, nil
	}}
	err := run(t.Context(), []string{"--port=/dev/cu.fake", "--baud=9600", "--scan-supported", "--json", "--output=" + path}, &out, &stderr, deps)
	if err != nil || calls != 1 || !json.Valid(out.Bytes()) || strings.Contains(out.String(), vin) {
		t.Fatalf("report: %v %s", err, out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), vin) || !bytes.Equal(data, out.Bytes()) {
		t.Fatal("file/stdout mismatch or VIN leakage")
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Mode().Perm()&0077 != 0 {
		t.Fatal("report is not private")
	}
	err = run(t.Context(), []string{"--port=/dev/cu.fake", "--output=" + path}, &out, &stderr, deps)
	if !errors.Is(err, os.ErrExist) || calls != 1 {
		t.Fatalf("overwrote report or touched hardware: %v calls %d", err, calls)
	}
}
