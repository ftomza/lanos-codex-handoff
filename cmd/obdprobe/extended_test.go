package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestExtendedFlagsAreFiniteOnly(t *testing.T) {
	t.Parallel()
	o, err := parseFlags([]string{"--transport=ble", "--read-permanent", "--read-freeze-frame", "--headers"}, &bytes.Buffer{})
	if err != nil || !o.config.ReadPermanent || !o.config.ReadFreezeFrame || !o.config.Headers {
		t.Fatal("missing finite options", err)
	}
	for _, flag := range []string{"--read-permanent", "--read-freeze-frame", "--headers"} {
		for _, mode := range []string{"--record", "--list-ports"} {
			if _, err := parseFlags([]string{mode, flag}, &bytes.Buffer{}); err == nil {
				t.Fatal("accepted mixed mode", mode, flag)
			}
		}
	}
}

func TestExtendedOutputKeepsMILUnknownCauseAndSeparateResults(t *testing.T) {
	t.Parallel()
	r := probe.Report{Status: "partial", Capabilities: []probe.Capability{{PID: "01", Support: "supported", Status: "available", Measurement: elm327.Measurement{MIL: new(true), DTCCount: new(0)}}}, StoredDTCs: probe.DTCResult{Status: "available", Codes: []string{}}, PendingDTCs: &probe.DTCResult{Status: "available", Codes: []string{}}, PermanentDTCs: &probe.DTCResult{Status: "unsupported"}, FreezeFrame: &probe.FreezeFrameResult{Frame: 0, Status: "temporarily_unavailable"}}
	var out bytes.Buffer
	if err := showReport(&out, r, true); err != nil {
		t.Fatal(err)
	}
	for _, phrase := range []string{"Permanent Mode 0A DTCs: status=unsupported", "Freeze frame 00: status=temporarily_unavailable", "not determined"} {
		if !strings.Contains(out.String(), phrase) {
			t.Fatal("missing diagnostic distinction", phrase, out.String())
		}
	}
	if strings.Contains(out.String(), "permanent codes not queried") {
		t.Fatal("queried mode labelled unqueried", out.String())
	}
}

func TestExtendedCLIForwardsFlagsWithoutHardware(t *testing.T) {
	t.Parallel()
	called := false
	err := run(t.Context(), []string{"--port=/dev/cu.synthetic", "--read-permanent", "--read-freeze-frame", "--headers"}, &bytes.Buffer{}, &bytes.Buffer{}, dependencies{probe: func(_ context.Context, cfg probe.Config) (probe.Report, error) {
		called = true
		if !cfg.ReadPermanent || !cfg.ReadFreezeFrame || !cfg.Headers {
			t.Error("lost flags")
		}
		return probe.Report{}, nil
	}})
	if err != nil || !called {
		t.Fatal(called, err)
	}
}

func TestFreezeDTCIsNotLabelledUndeterminedEmptyCodes(t *testing.T) {
	t.Parallel()
	r := probe.Report{Capabilities: []probe.Capability{{Status: "available", Measurement: elm327.Measurement{MIL: new(true)}}}, FreezeFrame: &probe.FreezeFrameResult{Status: "available", Readings: []probe.FreezeReading{{Status: "available", Codes: []string{"P0133"}}}}}
	if unresolvedMIL(r) {
		t.Fatal("freeze-frame cause code ignored")
	}
}
