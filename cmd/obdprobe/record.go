package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
)

func runRecording(ctx context.Context, o options, stdout, stderr io.Writer, deps dependencies) error {
	file, err := record.Open(o.output, o.redact)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "Read-only trip recording: %s\nDuration limit: %s; additional recovery attempts: %d; file limit: 256 MiB\n", file.Path(), o.duration, o.config.Retries); err != nil {
		return errors.Join(err, file.Close())
	}
	if deps.record == nil {
		deps.record = recordTrip
	}
	progress := recordProgress{out: stderr, redact: o.redact}
	summary, recordErr := deps.record(ctx, o.config, o.duration, func(event record.Event) error {
		if err := file.Write(event); err != nil {
			return err
		}
		return progress.show(event)
	})
	closeErr := file.Close()
	_, displayErr := fmt.Fprintf(stdout, "Recording ended: %s; elapsed=%s commands=%d samples=%d errors=%d gaps=%d; file=%s\n", summary.Reason, time.Duration(summary.DurationNS), summary.Commands, summary.Samples, summary.Errors, summary.Gaps, file.Path())
	return errors.Join(recordErr, closeErr, displayErr)
}

func recordTrip(ctx context.Context, cfg probe.Config, duration time.Duration, sink func(record.Event) error) (probe.RecordingSummary, error) {
	runner, err := newRunner(cfg)
	if err != nil {
		return probe.RecordingSummary{}, err
	}
	return runner.Record(ctx, duration, sink)
}

type recordProgress struct {
	out                       io.Writer
	redact                    bool
	lastNS                    int64
	commands, samples, errors int64
}

func (p *recordProgress) show(event record.Event) error {
	var message string
	switch event.Type {
	case "capabilities":
		if data, ok := event.Data.(probe.RecordingCapabilities); ok {
			message = fmt.Sprintf("Recording active; segment=%d source=%s missing fuel-model inputs=%v", event.Segment, data.Report.Source, data.MissingInputs)
		}
	case "gap", "recovery", "segment_start":
		message = fmt.Sprintf("%s segment=%d: %v", event.Type, event.Segment, event.Data)
	case "command":
		p.commands++
		if data, ok := event.Data.(probe.RecordingCommand); ok {
			if data.Result != nil && (data.Result.Status == "available" || data.Result.Status == "raw_only") {
				p.samples++
			}
			if data.Error != "" || data.Status != "ok" || data.Result != nil && (data.Result.Status == "invalid_response" || data.Result.Status == "temporarily_unavailable") {
				p.errors++
				// Coalesce repeated NO DATA/errors; the complete evidence stays in JSONL.
				if p.errors == 1 {
					message = fmt.Sprintf("Command %s unavailable: status=%s error=%s", data.Command, data.Status, data.Error)
				}
			}
		}
	}
	if message == "" && event.ElapsedNS-p.lastNS >= int64(10*time.Second) {
		message = fmt.Sprintf("Recording %s; commands=%d samples=%d errors=%d segment=%d", time.Duration(event.ElapsedNS).Round(time.Second), p.commands, p.samples, p.errors, event.Segment)
		p.lastNS = event.ElapsedNS
	}
	if message == "" {
		return nil
	}
	if p.redact {
		message = privacy.VIN(message)
	}
	_, err := fmt.Fprintln(p.out, message)
	return err
}
