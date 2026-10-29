package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

type options struct {
	config             probe.Config
	list, json, redact bool
	output             string
	record             bool
	duration           time.Duration
}

func parseFlags(args []string, stderr io.Writer) (options, error) {
	o := options{}
	flags := flag.NewFlagSet("obdprobe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cfg := &o.config
	flags.StringVar(&cfg.Transport, "transport", "serial", "transport: serial (SPP) or ble (native GATT)")
	flags.StringVar(&cfg.BLEName, "ble-name", "IOS-Vlink", "explicit BLE advertised device name")
	flags.StringVar(&cfg.BLEHelper, "ble-helper", "", "native helper path; default is obdprobe-ble beside obdprobe")
	connectTimeout := flags.Duration("ble-connect-timeout", 30*time.Second, "BLE discovery/connection deadline, separate from ELM command timeout")
	flags.StringVar(&cfg.AdapterModel, "adapter-model", probe.KnownAdapterModel, "contextual device model, not ATI detection; empty means unknown")
	flags.StringVar(&cfg.Port, "port", "", "explicit macOS /dev/cu.* port; never selected automatically")
	flags.IntVar(&cfg.Baud, "baud", 38400, "serial baud: 38400 or 9600")
	flags.DurationVar(&cfg.Timeout, "timeout", 3*time.Second, "ELM command timeout; also serial open timeout")
	flags.DurationVar(&cfg.InitialTimeout, "initial-timeout", 30*time.Second, "initial 0100 protocol search timeout")
	flags.DurationVar(&cfg.RecoveryTimeout, "recovery-timeout", 30*time.Second, "read-only wait for delayed prompt after timeout/disconnect")
	flags.IntVar(&cfg.Retries, "retries", 2, "additional attempts after transport failures (0..5)")
	flags.BoolVar(&o.record, "record", false, "record a private capability-gated trip JSONL stream")
	flags.DurationVar(&o.duration, "duration", 4*time.Hour, "recording duration, >0 and <=24h; requires --record")
	flags.BoolVar(&cfg.ScanSupported, "scan-supported", false, "discover continuation bitmaps and read advertised Mode 01 PIDs")
	flags.BoolVar(&cfg.ReadPending, "read-pending", false, "finite probe only: additionally read pending DTCs with Mode 07; never clears codes")
	flags.BoolVar(&o.list, "list-ports", false, "enumerate /dev/cu.* names without opening any port")
	flags.BoolVar(&o.json, "json", false, "write one machine-readable JSON report to stdout")
	flags.StringVar(&o.output, "output", "", "private report JSON or recording JSONL; default recording path: diagnostics/trip-*.jsonl; never overwrite")
	flags.BoolVar(&o.redact, "redact-vin", true, "redact recognizable unsolicited VIN data; VIN is never requested")
	if err := flags.Parse(args); err != nil {
		return o, err
	}
	if flags.NArg() != 0 || cfg.Timeout <= 0 || cfg.InitialTimeout <= 0 || cfg.RecoveryTimeout <= 0 || cfg.Retries < 0 || cfg.Retries > 5 || (cfg.Baud != 9600 && cfg.Baud != 38400) {
		return o, fmt.Errorf("invalid flags: positive timeouts, retries 0..5, serial baud 9600/38400 and no positional arguments required")
	}
	explicit := make(map[string]bool, flags.NFlag())
	flags.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if cfg.ReadPending && (o.record || o.list) {
		return o, fmt.Errorf("read-pending requires a finite diagnostic probe")
	}
	if o.record && (o.json || o.list || cfg.ScanSupported || o.duration <= 0 || o.duration > 24*time.Hour) {
		return o, fmt.Errorf("record requires duration in (0,24h] and cannot combine with json, list-ports or scan-supported")
	}
	if explicit["duration"] && !o.record {
		return o, fmt.Errorf("duration requires --record")
	}
	switch cfg.Transport {
	case "serial":
		for _, name := range []string{"ble-name", "ble-helper", "ble-connect-timeout"} {
			if explicit[name] {
				return o, fmt.Errorf("%s requires --transport=ble", name)
			}
		}
	case "ble":
		if explicit["port"] || explicit["baud"] || o.list {
			return o, fmt.Errorf("ble cannot be combined with serial port, baud or list-ports")
		}
		if *connectTimeout <= 0 || strings.TrimSpace(cfg.BLEName) == "" || len(cfg.BLEName) > 128 || strings.ContainsFunc(cfg.BLEName, unicode.IsControl) {
			return o, fmt.Errorf("invalid BLE name or connection timeout")
		}
		cfg.Baud = 0
		cfg.ConnectTimeout = *connectTimeout
	default:
		return o, fmt.Errorf("transport must be serial or ble")
	}
	return o, nil
}
