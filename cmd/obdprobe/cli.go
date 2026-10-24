package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/ble"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/serial"
)

const portGuidance = "If no /dev/cu.* port appears, macOS Bluetooth Classic SPP is unavailable for this connection. Use the native BLE path with --transport=ble after make build-obdprobe, or reconnect SPP manually. Missing SPP does not prove ECU incompatibility."

type dependencies struct {
	listPorts func() ([]string, error)
	probe     func(context.Context, probe.Config) (probe.Report, error)
	record    func(context.Context, probe.Config, time.Duration, func(record.Event) error) (probe.RecordingSummary, error)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, deps dependencies) error {
	options, err := parseFlags(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if options.list {
		if options.config.Port != "" || options.output != "" {
			return fmt.Errorf("list-ports cannot be combined with port or output")
		}
		if deps.listPorts == nil {
			deps.listPorts = serial.ListPorts
		}
		return showPorts(stdout, options.json, deps.listPorts)
	}
	if options.config.Transport == "serial" && options.config.Port == "" {
		return fmt.Errorf("no port selected; run obdprobe --list-ports, then supply --port. %s", portGuidance)
	}
	if options.record {
		return runRecording(ctx, options, stdout, stderr, deps)
	}
	var file *os.File
	if options.output != "" {
		var err error
		file, err = os.OpenFile(options.output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return fmt.Errorf("create diagnostic report: %w", err)
		}
	}
	if deps.probe == nil {
		deps.probe = runProbe
	}
	report, probeErr := deps.probe(ctx, options.config)
	data, err := encodeReport(report, options.redact)
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
	if options.json {
		_, displayErr = stdout.Write(data)
	} else {
		displayErr = showReport(stdout, report, options.redact)
	}
	return errors.Join(probeErr, fileErr, displayErr)
}

func newRunner(cfg probe.Config) (*probe.Runner, error) {
	return probe.New(cfg, func(ctx context.Context) (elm327.Transport, error) {
		if cfg.Transport == "ble" {
			return ble.Open(ctx, ble.Config{Name: cfg.BLEName, Helper: cfg.BLEHelper})
		}
		return serial.Open(ctx, cfg.Port, cfg.Baud)
	})
}

func runProbe(ctx context.Context, cfg probe.Config) (probe.Report, error) {
	runner, err := newRunner(cfg)
	if err != nil {
		return probe.Report{}, err
	}
	return runner.Run(ctx)
}
