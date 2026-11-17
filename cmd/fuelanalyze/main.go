// Command fuelanalyze analyzes saved diagnostic history without opening hardware.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuelhistory"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("fuelanalyze", flag.ContinueOnError)
	f.SetOutput(out)
	input := f.String("input", "", "saved fuel history directory (no ECU access)")
	session := f.String("session", "", "optional saved session id")
	asJSON := f.Bool("json", false, "emit versioned report JSON, including bounded saved series")
	from := f.String("from", "", "optional RFC3339 start time")
	to := f.String("to", "", "optional RFC3339 end time")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *input == "" || f.NArg() != 0 {
		return fmt.Errorf("explicit --input and no positional arguments required")
	}
	options := fuelhistory.Options{Session: *session}
	for _, item := range []struct {
		value  string
		target *time.Time
	}{{*from, &options.From}, {*to, &options.To}} {
		if item.value != "" {
			parsed, err := time.Parse(time.RFC3339Nano, item.value)
			if err != nil {
				return err
			}
			*item.target = parsed
		}
	}
	if err := options.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	report, err := fuelhistory.Analyze(ctx, *input, options)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(report)
	}
	return text(out, report)
}
func number(v *float64) string {
	if v == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.3f", *v)
}
func text(out io.Writer, r fuelhistory.Report) error {
	if _, err := fmt.Fprintf(out, "Fuel history v%d — %s\nRange: %s — %s\nObserved duration: %.1f s; distance: %.4f km\nEstimated fuel: %.5f L; covered average: %s L/100km\nObserved speed average/max: %s / %s km/h\nCoverage: %.2f%%; gaps: %.2f s; incomplete: %t\nSampled fuel comparison: %.5f L\nScope: %s\n", r.Version, r.Source, r.From.UTC().Format(time.RFC3339), r.To.UTC().Format(time.RFC3339), r.DurationSeconds, r.DistanceKM, r.Liters, number(r.AverageL100), number(r.AverageSpeed), number(r.MaxSpeed), r.CoveragePercent, r.GapSeconds, r.Incomplete, r.SampledLiters, r.CoverageScope); err != nil {
		return err
	}
	for _, key := range []string{"idle", "moving", "engine_off", "unknown"} {
		s := r.Modes[key]
		if _, err := fmt.Fprintf(out, "%s: %.1f s, %.5f L, %.4f km\n", key, s.Seconds, s.Liters, s.DistanceKM); err != nil {
			return err
		}
	}
	for _, a := range r.Anomalies {
		if _, err := fmt.Fprintf(out, "IAT diagnostic: %s at %s (not a sensor diagnosis)\n", a.Code, a.At.UTC().Format(time.RFC3339)); err != nil {
			return err
		}
	}
	for _, issue := range r.Issues {
		if _, err := fmt.Fprintf(out, "History issue: %s %s\n", issue.Code, issue.Detail); err != nil {
			return err
		}
	}
	return nil
}
