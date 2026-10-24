package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestBLEFlagsAndNoImplicitFallback(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--transport=ble", "--json"},
		{"--transport=ble", "--ble-name=IOS-Vlink", "--ble-connect-timeout=20s", "--ble-helper=/test/obdprobe-ble"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			called := false
			err := run(t.Context(), args, &out, &stderr, dependencies{probe: func(ctx context.Context, cfg probe.Config) (probe.Report, error) {
				called = true
				if cfg.Transport != "ble" || cfg.BLEName != "IOS-Vlink" || cfg.Port != "" || cfg.Baud != 0 || cfg.ConnectTimeout < 20*time.Second {
					t.Fatalf("config: %+v", cfg)
				}
				return probe.Report{Transport: "ble_gatt", Status: "complete"}, nil
			}})
			if err != nil || !called {
				t.Fatalf("BLE invocation: %v called=%t stderr=%s", err, called, stderr.String())
			}
		})
	}
}

func TestInvalidTransportFlagsNeverOpenHardware(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--transport=guess"}, {"--transport=ble", "--port=/dev/cu.fake"}, {"--transport=ble", "--baud=38400"},
		{"--transport=ble", "--list-ports"}, {"--transport=ble", "--ble-name="}, {"--transport=ble", "--ble-name=\n"},
		{"--transport=ble", "--ble-connect-timeout=0s"}, {"--ble-name=IOS-Vlink", "--port=/dev/cu.fake"},
		{"--ble-helper=/tmp/helper", "--port=/dev/cu.fake"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			called := false
			err := run(t.Context(), args, &out, &stderr, dependencies{probe: func(context.Context, probe.Config) (probe.Report, error) { called = true; return probe.Report{}, nil }})
			if err == nil || called {
				t.Fatalf("unsafe flags: %v err=%v called=%t", args, err, called)
			}
		})
	}
}

func TestBLEFailureExportsWithoutSerialFallback(t *testing.T) {
	t.Parallel()
	var out, stderr bytes.Buffer
	cause := errors.New("native backend missing")
	err := run(t.Context(), []string{"--transport=ble", "--json"}, &out, &stderr, dependencies{probe: func(context.Context, probe.Config) (probe.Report, error) {
		return probe.Report{Transport: "ble_gatt", Status: "failed", Error: cause.Error()}, cause
	}})
	if !errors.Is(err, cause) || !strings.Contains(out.String(), "ble_gatt") || !strings.Contains(out.String(), cause.Error()) {
		t.Fatalf("failure lost: %v %s", err, out.String())
	}
}
