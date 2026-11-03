package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestExtendedFlagsFiniteOnly(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--read-permanent", "--read-freeze-frame", "--headers"} {
		t.Run(flag, func(t *testing.T) {
			t.Parallel()
			o, err := parseFlags([]string{"--transport=ble", flag}, &bytes.Buffer{})
			if err != nil || !(o.config.ReadPermanent || o.config.ReadFreezeFrame || o.config.Headers) {
				t.Fatal(o, err)
			}
			for _, mode := range []string{"--record", "--list-ports"} {
				if _, err := parseFlags([]string{mode, flag}, &bytes.Buffer{}); err == nil {
					t.Fatal("accepted", mode, flag)
				}
			}
		})
	}
}

func TestExtendedCLISeparatesEvidenceAndUnresolvedMIL(t *testing.T) {
	t.Parallel()
	mil, count := true, 0
	var out bytes.Buffer
	err := run(t.Context(), []string{"--port=/dev/cu.synthetic", "--read-permanent", "--read-freeze-frame", "--headers"}, &out, &bytes.Buffer{}, dependencies{probe: func(_ context.Context, cfg probe.Config) (probe.Report, error) {
		if !cfg.ReadPermanent || !cfg.ReadFreezeFrame || !cfg.Headers {
			t.Error("flags lost")
		}
		return probe.Report{Capabilities: []probe.Capability{{Status: "available", Measurement: elm327.Measurement{MIL: &mil, DTCCount: &count}}}, PermanentDTCs: &probe.DTCResult{Status: "unsupported"}, FreezeFrame: &probe.FreezeFrameResult{Status: "available", Readings: []probe.FreezeReading{{PID: "0C", Status: "available"}}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Permanent Mode 0A DTCs: status=unsupported", "Freeze frame 00: status=available", "Freeze PID 0C", "cause is not determined by queried standard diagnostic modes"} {
		if !strings.Contains(out.String(), want) {
			t.Fatal(want, out.String())
		}
	}
	if strings.Contains(out.String(), "permanent codes not queried") {
		t.Fatal(out.String())
	}
}
