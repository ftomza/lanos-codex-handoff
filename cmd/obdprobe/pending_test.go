package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestPendingFlagRequiresFiniteProbe(t *testing.T) {
	t.Parallel()
	o, err := parseFlags([]string{"--transport=ble", "--read-pending"}, &bytes.Buffer{})
	if err != nil || !o.config.ReadPending {
		t.Fatal("pending flag lost", err)
	}
	for _, args := range [][]string{{"--record", "--read-pending"}, {"--list-ports", "--read-pending"}} {
		if _, err := parseFlags(args, &bytes.Buffer{}); err == nil {
			t.Fatal("mixed modes accepted", args)
		}
	}
}

func TestPendingCLIReportsReadSeparately(t *testing.T) {
	t.Parallel()
	for _, asJSON := range []bool{false, true} {
		t.Run(fmt.Sprint(asJSON), func(t *testing.T) {
			t.Parallel()
			args := []string{"--port=/dev/cu.synthetic", "--read-pending"}
			if asJSON {
				args = append(args, "--json")
			}
			var out bytes.Buffer
			err := run(t.Context(), args, &out, &bytes.Buffer{}, dependencies{probe: func(_ context.Context, cfg probe.Config) (probe.Report, error) {
				if !cfg.ReadPending {
					t.Error("flag not forwarded")
				}
				return probe.Report{Status: "complete", StoredDTCs: probe.DTCResult{Status: "available", Codes: []string{}}, PendingDTCs: &probe.DTCResult{Source: "ecu_mode07", Status: "available", Codes: []string{"P0133"}}}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			if asJSON {
				var report probe.Report
				if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.PendingDTCs == nil || !slices.Equal(report.PendingDTCs.Codes, []string{"P0133"}) || len(report.StoredDTCs.Codes) != 0 {
					t.Fatal("lost separate pending report", err, out.String())
				}
				return
			}
			if !strings.Contains(out.String(), "Pending Mode 07 DTCs: status=available codes=[P0133]") || strings.Contains(out.String(), "pending/permanent codes not queried") {
				t.Fatal("misleading output", out.String())
			}
		})
	}
}
