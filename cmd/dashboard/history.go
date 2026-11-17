package main

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuelhistory"
)

func registerHistoryFlags(f *flag.FlagSet, o *dashboardOptions) {
	f.StringVar(&o.recordDir, "fuel-record-dir", "", "opt-in bounded diagnostic JSONL directory; separate SIM/LIVE")
	f.StringVar(&o.historyDir, "fuel-history-dir", "", "read existing diagnostic history without enabling recording")
	f.DurationVar(&o.recordInterval, "fuel-record-interval", time.Second, "diagnostic sample interval: 250ms..2s")
	f.Int64Var(&o.recordMaxBytes, "fuel-record-max-bytes", 16<<20, "maximum bytes per diagnostic part: 1..64MiB")
	f.IntVar(&o.recordMaxFiles, "fuel-record-max-files", 8, "maximum managed parts including active: 1..32")
	f.BoolVar(&o.recordCold, "fuel-record-cold-start", false, "owner annotation of a cold engine; never inferred")
}
func validateHistoryFlags(o *dashboardOptions, explicit map[string]bool) error {
	if o.recordDir == "" {
		for _, name := range []string{"fuel-record-dir", "fuel-record-interval", "fuel-record-max-bytes", "fuel-record-max-files", "fuel-record-cold-start"} {
			if explicit[name] {
				return fmt.Errorf("%s requires nonempty fuel-record-dir", name)
			}
		}
	}
	if o.recordDir != "" && o.fuelMode == "off" {
		return fmt.Errorf("fuel recording requires an enabled fuel computer")
	}
	if o.recordInterval < 250*time.Millisecond || o.recordInterval > 2*time.Second {
		return fmt.Errorf("fuel record interval must be 250ms..2s")
	}
	if o.recordMaxBytes < 1<<20 || o.recordMaxBytes > 64<<20 || o.recordMaxFiles < 1 || o.recordMaxFiles > 32 {
		return fmt.Errorf("invalid fuel recording limits")
	}
	for _, path := range []*string{&o.recordDir, &o.historyDir} {
		if *path != "" {
			absolute, err := filepath.Abs(*path)
			if err != nil {
				return err
			}
			*path = absolute
		}
	}
	if o.recordDir != "" {
		if o.historyDir != "" && o.historyDir != o.recordDir {
			return fmt.Errorf("record and history directories must match")
		}
		o.historyDir = o.recordDir
	}
	return nil
}
func openHistory(o dashboardOptions, source string, c *fuel.Computer) (*fuelhistory.Recorder, string, error) {
	if o.recordDir == "" {
		if o.historyDir != "" {
			if _, err := fuelhistory.List(o.historyDir); err != nil {
				return nil, "", err
			}
		}
		return nil, o.historyDir, nil
	}
	if c == nil {
		return nil, "", fmt.Errorf("fuel recorder needs a computer")
	}
	cfg := fuelhistory.DefaultConfig(o.recordDir, source)
	cfg.MaxBytes, cfg.MaxFiles = o.recordMaxBytes, o.recordMaxFiles
	r, err := fuelhistory.Open(cfg)
	if err != nil {
		return nil, "", err
	}
	if err := c.SetDiagnosticObserver(o.recordInterval, o.recordCold, r.Submit); err != nil {
		closeErr := r.Close()
		return nil, "", errors.Join(err, closeErr)
	}
	return r, o.historyDir, nil
}
func closeHistory(r *fuelhistory.Recorder) error {
	if r == nil {
		return nil
	}
	return r.Close()
}
