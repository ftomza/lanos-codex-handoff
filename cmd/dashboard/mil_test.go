package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
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

func TestMILHistoryFlagsReadOnlyNormalization(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "missing-mil")
	for _, source := range []string{"sim", "elm327"} {
		o, err := parseDashboardFlags([]string{"--source=" + source, "--mil-history-dir=" + dir, "--fuel=off"}, io.Discard)
		if err != nil || o.milHistoryDir != dir || o.milDir != "" || o.probe.MILInterval != 0 {
			t.Fatalf("options=%+v err=%v", o, err)
		}
		capture, err := openMIL(o, "live", nil)
		if err != nil || capture != nil {
			t.Fatal("history enabled capture", capture, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("history created directory", err)
		}
	}
	o, err := parseDashboardFlags([]string{"--mil-record-dir=data/mil", "--mil-history-dir=./data/mil"}, io.Discard)
	if err != nil || o.milDir != o.milHistoryDir || !filepath.IsAbs(o.milHistoryDir) {
		t.Fatal(o, err)
	}
	o, err = parseDashboardFlags([]string{"--mil-record-dir=data/mil"}, io.Discard)
	if err != nil || o.milDir != o.milHistoryDir {
		t.Fatal(o, err)
	}
}

func TestMILHistoryFlagsRejectMixedAndFuelPaths(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--mil-history-dir="}, {"--mil-history-dir= "},
		{"--mil-record-dir=mil", "--mil-history-dir=other"},
		{"--mil-record-dir=mil", "--mil-history-dir="},
		{"--mil-history-dir=mil", "--mil-interval=5s"},
		{"--mil-history-dir=x", "--fuel-record-dir=./x"},
		{"--mil-history-dir=x", "--fuel-history-dir=./x"},
		{"--mil-record-dir=x", "--fuel-history-dir=./x"},
	} {
		if _, err := parseDashboardFlags(args, io.Discard); err == nil {
			t.Fatal("accepted incompatible paths/options", args)
		}
	}
}

func TestMILHistoryMissingDirectoryDoesNotStopDashboard(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "missing")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	bound := make(chan net.Listener, 1)
	done := make(chan error, 1)
	deps := dashboardDependencies{listen: func(ctx context.Context, network, address string) (net.Listener, error) {
		l, err := (&net.ListenConfig{}).Listen(ctx, network, "127.0.0.1:0")
		if err == nil {
			bound <- l
		}
		return l, err
	}}
	go func() {
		done <- runWith(ctx, []string{"--mil-history-dir=" + dir, "--fuel=off"}, &bytes.Buffer{}, deps)
	}()
	var listener net.Listener
	select {
	case listener = <-bound:
	case err := <-done:
		t.Fatal("startup failed", err)
	case <-time.After(2 * time.Second):
		t.Fatal("listener timeout")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, selection := range []struct {
		path string
		code int
	}{{"/api/snapshot", 200}, {"/api/mil/history/sessions", 409}} {
		resp, err := client.Get("http://" + listener.Addr().String() + selection.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != selection.code {
			t.Fatalf("%s: %d %s %v", selection.path, resp.StatusCode, body, err)
		}
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("read-only startup created directory", err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown timeout")
	}
}
