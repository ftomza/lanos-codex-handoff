package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

func registerMILFlags(f *flag.FlagSet, o *dashboardOptions) {
	f.StringVar(&o.milDir, "mil-record-dir", "", "opt-in private MIL event/context JSONL directory; separate SIM/LIVE")
	f.DurationVar(&o.milInterval, "mil-interval", 5*time.Second, "MIL 0101 polling target 1s..60s; requires mil-record-dir")
}

func validateMILFlags(o *dashboardOptions, explicit map[string]bool) error {
	if o.milDir == "" {
		if explicit["mil-interval"] || explicit["mil-record-dir"] {
			return fmt.Errorf("mil options require nonempty mil-record-dir")
		}
		return nil
	}
	if strings.TrimSpace(o.milDir) == "" || o.milInterval < time.Second || o.milInterval > time.Minute {
		return fmt.Errorf("mil requires a directory and interval 1s..60s")
	}
	abs, err := filepath.Abs(o.milDir)
	if err != nil {
		return err
	}
	if abs == o.recordDir || abs == o.historyDir {
		return fmt.Errorf("mil and fuel history require separate directories")
	}
	o.milDir = abs
	if o.source == "elm327" {
		o.probe.MILInterval = o.milInterval
	}
	return nil
}

func openMIL(o dashboardOptions, source string, computer *fuel.Computer) (*mil.Capture, error) {
	if o.milDir == "" {
		return nil, nil
	}
	w, err := milstore.Open(milstore.DefaultConfig(o.milDir, source))
	if err != nil {
		return nil, err
	}
	var view func(time.Time) (json.RawMessage, error)
	if computer != nil {
		view = func(now time.Time) (json.RawMessage, error) {
			return json.Marshal(computer.Diagnostic(now))
		}
	}
	return mil.NewCapture(w, view, o.staleAfter), nil
}

func closeMIL(c *mil.Capture) error {
	if c == nil {
		return nil
	}
	return c.Close()
}
