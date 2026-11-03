package main

import (
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestMILFlagsOptInAndEarlyValidation(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--mil-interval=5s"}, {"--mil-record-dir="}, {"--mil-record-dir=x", "--mil-interval=0s"}, {"--mil-record-dir=x", "--mil-interval=61s"}, {"--mil-record-dir=x", "--fuel-record-dir=x"}} {
		if _, err := parseDashboardFlags(args, io.Discard); err == nil {
			t.Fatal("accepted invalid MIL flags", args)
		}
	}
	o, err := parseDashboardFlags(nil, io.Discard)
	if err != nil || o.probe.MILInterval != 0 || o.milDir != "" {
		t.Fatal(o, err)
	}
	o, err = parseDashboardFlags([]string{"--source=elm327", "--mil-record-dir=data/mil-events/live"}, io.Discard)
	if err != nil || o.probe.MILInterval != 5*time.Second || !filepath.IsAbs(o.milDir) {
		t.Fatal(o, err)
	}
	// Synthetic sources can record UNKNOWN; no fake MIL is injected by this flag.
	o, err = parseDashboardFlags([]string{"--mil-record-dir=data/mil-events/sim", "--fuel=off"}, io.Discard)
	if err != nil || o.milDir == "" {
		t.Fatal(o, err)
	}
}
