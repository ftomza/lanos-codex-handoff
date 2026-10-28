package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

type dashboardOptions struct {
	fuelMode, fuelState  string
	fuelReset            bool
	fuelConfig           fuel.Config
	source, listen       string
	interval, staleAfter time.Duration
	probe                probe.Config
}

func parseDashboardFlags(args []string, out io.Writer) (dashboardOptions, error) {
	o := dashboardOptions{}
	f := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	f.SetOutput(out)
	f.StringVar(&o.source, "source", "sim", "telemetry source: sim or explicit elm327 (live vehicle)")
	f.StringVar(&o.listen, "listen", "127.0.0.1:8080", "HTTP address; 0.0.0.0:8080 for trusted LAN access")
	f.DurationVar(&o.interval, "interval", 250*time.Millisecond, "simulator sample interval; sim only")
	f.DurationVar(&o.staleAfter, "stale-after", 3*time.Second, "global silence expiry; individual live signals also have independent TTLs")
	c := &o.probe
	f.StringVar(&c.Transport, "transport", "ble", "live transport: ble or serial; requires --source=elm327")
	f.StringVar(&c.BLEName, "ble-name", "IOS-Vlink", "exact BLE advertised device name")
	f.StringVar(&c.BLEHelper, "ble-helper", "", "native helper path; default obdprobe-ble beside dashboard")
	f.DurationVar(&c.ConnectTimeout, "ble-connect-timeout", 30*time.Second, "BLE discovery/connect deadline")
	f.StringVar(&c.Port, "port", "", "explicit macOS /dev/cu.* for serial")
	f.IntVar(&c.Baud, "baud", 38400, "serial baud: 38400 or 9600")
	f.DurationVar(&c.Timeout, "timeout", 3*time.Second, "ELM command timeout")
	f.DurationVar(&c.InitialTimeout, "initial-timeout", 30*time.Second, "initial ECU protocol search timeout")
	f.DurationVar(&c.RecoveryTimeout, "recovery-timeout", 30*time.Second, "read-only wait for a delayed prompt")
	f.IntVar(&c.Retries, "retries", 2, "additional attempts per live reconnect burst (0..5); retry after 10s idle even with 0")
	c.AdapterModel = probe.KnownAdapterModel
	registerFuelFlags(f, &o)
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 || strings.TrimSpace(o.listen) == "" || o.staleAfter <= 0 {
		return o, fmt.Errorf("invalid configuration: no positional arguments, nonempty listen and positive stale-after required")
	}
	explicit := make(map[string]bool, f.NFlag())
	f.Visit(func(flag *flag.Flag) { explicit[flag.Name] = true })
	if err := validateDashboard(&o, explicit); err != nil {
		return o, err
	}
	if err := validateFuelFlags(&o); err != nil {
		return o, err
	}
	return o, nil
}

func validateDashboard(o *dashboardOptions, explicit map[string]bool) error {
	if o.source == "sim" {
		for _, name := range []string{"transport", "ble-name", "ble-helper", "ble-connect-timeout", "port", "baud", "timeout", "initial-timeout", "recovery-timeout", "retries"} {
			if explicit[name] {
				return fmt.Errorf("%s requires --source=elm327", name)
			}
		}
		if o.interval < 50*time.Millisecond || o.staleAfter <= o.interval {
			return fmt.Errorf("sim requires interval >=50ms and stale-after > interval")
		}
		return nil
	}
	if o.source != "elm327" {
		return fmt.Errorf("source must be sim or elm327")
	}
	if explicit["interval"] {
		return fmt.Errorf("interval is simulator-only; live uses capability-gated polling")
	}
	c := &o.probe
	if c.Timeout <= 0 || c.InitialTimeout <= 0 || c.RecoveryTimeout <= 0 || c.ConnectTimeout <= 0 || c.Retries < 0 || c.Retries > 5 {
		return fmt.Errorf("positive timeouts and retries 0..5 required")
	}
	if c.Transport == "ble" {
		if explicit["port"] || explicit["baud"] {
			return fmt.Errorf("ble cannot combine with serial port or baud")
		}
		if strings.TrimSpace(c.BLEName) == "" || len(c.BLEName) > 128 || strings.ContainsFunc(c.BLEName, unicode.IsControl) {
			return fmt.Errorf("invalid BLE name")
		}
		c.Baud = 0
		return nil
	}
	if c.Transport != "serial" {
		return fmt.Errorf("transport must be ble or serial")
	}
	for _, name := range []string{"ble-name", "ble-helper", "ble-connect-timeout"} {
		if explicit[name] {
			return fmt.Errorf("%s requires BLE transport", name)
		}
	}
	if !strings.HasPrefix(c.Port, "/dev/cu.") || filepath.Clean(c.Port) != c.Port || strings.Contains(strings.TrimPrefix(c.Port, "/dev/cu."), "/") || (c.Baud != 38400 && c.Baud != 9600) {
		return fmt.Errorf("serial requires explicit /dev/cu.* port and baud 38400/9600")
	}
	c.ConnectTimeout = c.Timeout
	return nil
}
