package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/serial"
)

const portGuidance = "If no /dev/cu.* port appears, macOS Bluetooth Classic SPP is unavailable for this connection. Reconnect/pair Vgate manually, or use an Android native SPP client / a compatible USB K-Line adapter. Missing SPP does not prove ECU incompatibility."

type dependencies struct {
	listPorts func() ([]string, error)
	probe     func(context.Context, probe.Config) (probe.Report, error)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) error {
	flags := flag.NewFlagSet("obdprobe", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cfg := probe.Config{}
	flags.StringVar(&cfg.AdapterModel, "adapter-model", probe.KnownAdapterModel, "contextual device model, not ATI detection; empty means unknown")
	flags.StringVar(&cfg.Port, "port", "", "explicit macOS /dev/cu.* port; never selected automatically")
	flags.IntVar(&cfg.Baud, "baud", 38400, "baud rate: 38400 or 9600 (Issue #1 did not record the baud)")
	flags.DurationVar(&cfg.Timeout, "timeout", 3*time.Second, "per-command/open timeout")
	flags.DurationVar(&cfg.InitialTimeout, "initial-timeout", 30*time.Second, "initial 0100 protocol search timeout")
	flags.DurationVar(&cfg.RecoveryTimeout, "recovery-timeout", 30*time.Second, "read-only wait for delayed prompt after timeout/disconnect")
	flags.IntVar(&cfg.Retries, "retries", 2, "additional attempts after transport failures (0..5)")
	flags.BoolVar(&cfg.ScanSupported, "scan-supported", false, "discover continuation bitmaps and read advertised Mode 01 PIDs")
	list := flags.Bool("list-ports", false, "enumerate /dev/cu.* names without opening any port")
	jsonOutput := flags.Bool("json", false, "write one machine-readable JSON report to stdout")
	output := flags.String("output", "", "create a private JSON report; an existing file is never overwritten")
	redact := flags.Bool("redact-vin", true, "redact recognizable unsolicited VIN data; VIN is never requested")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || cfg.Timeout <= 0 || cfg.InitialTimeout <= 0 || cfg.RecoveryTimeout <= 0 || cfg.Retries < 0 || cfg.Retries > 5 || (cfg.Baud != 9600 && cfg.Baud != 38400) {
		return fmt.Errorf("invalid flags: positive timeouts, retries 0..5, baud 9600 or 38400 and no positional arguments required")
	}
	if *list {
		if cfg.Port != "" || *output != "" {
			return fmt.Errorf("list-ports cannot be combined with port or output")
		}
		if deps.listPorts == nil {
			deps.listPorts = serial.ListPorts
		}
		return showPorts(stdout, *jsonOutput, deps.listPorts)
	}
	if cfg.Port == "" {
		return fmt.Errorf("no port selected; run obdprobe --list-ports, then supply --port. %s", portGuidance)
	}
	var file *os.File
	if *output != "" {
		var err error
		file, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create diagnostic report: %w", err)
		}
	}
	if deps.probe == nil {
		deps.probe = runProbe
	}
	report, probeErr := deps.probe(ctx, cfg)
	data, err := encodeReport(report, *redact)
	if err != nil {
		if file != nil {
			err = errors.Join(err, file.Close())
		}
		return errors.Join(probeErr, err)
	}
	var fileErr error
	if file != nil {
		_, fileErr = file.Write(data)
		fileErr = errors.Join(fileErr, file.Close())
	}
	var displayErr error
	if *jsonOutput {
		_, displayErr = stdout.Write(data)
	} else {
		displayErr = showReport(stdout, report, *redact)
	}
	return errors.Join(probeErr, fileErr, displayErr)
}

func runProbe(ctx context.Context, cfg probe.Config) (probe.Report, error) {
	runner, err := probe.New(cfg, func(ctx context.Context) (elm327.Transport, error) { return serial.Open(ctx, cfg.Port, cfg.Baud) })
	if err != nil {
		return probe.Report{}, err
	}
	return runner.Run(ctx)
}
