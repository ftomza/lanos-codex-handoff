// Package fuelhistory stores and analyzes bounded, opt-in diagnostic history.
package fuelhistory

import (
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

const Version = 1
const MaxLine = 64 << 10

type Config struct {
	Dir      string
	Source   string
	MaxBytes int64
	MaxFiles int
}

func DefaultConfig(dir, source string) Config {
	return Config{Dir: dir, Source: source, MaxBytes: 16 << 20, MaxFiles: 8}
}
func (c Config) Validate() error {
	if c.Dir == "" || c.Source != telemetry.Live && c.Source != telemetry.Simulator || c.MaxBytes < 1<<20 || c.MaxBytes > 64<<20 || c.MaxFiles < 1 || c.MaxFiles > 32 {
		return fmt.Errorf("invalid fuel history configuration")
	}
	return nil
}

type Entry struct {
	Version   int              `json:"version"`
	Session   string           `json:"session_id"`
	Sequence  uint64           `json:"sequence"`
	Kind      string           `json:"kind"`
	Source    string           `json:"source"`
	At        time.Time        `json:"at"`
	ElapsedNS int64            `json:"elapsed_ns"`
	Reason    string           `json:"reason,omitempty"`
	Error     string           `json:"error,omitempty"`
	Sample    *fuel.Diagnostic `json:"sample,omitempty"`
	// Boundary is reader metadata only: corrupt/truncated/filtered input cannot
	// become a contiguous interval merely because it produced no decoded row.
	Boundary      bool `json:"-"`
	effective     *telemetry.Snapshot
	effectiveFuel *telemetry.Fuel
}
type Status struct {
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
	Session string `json:"session_id"`
	Rows    uint64 `json:"rows"`
}
type Options struct {
	Session   string
	From      time.Time
	To        time.Time
	MaxPoints int
}

func (o Options) Validate() error {
	if o.Session != "" && !validSession(o.Session) {
		return fmt.Errorf("invalid history session")
	}
	if !o.From.IsZero() && !o.To.IsZero() && !o.To.After(o.From) {
		return fmt.Errorf("history end must be after start")
	}
	if o.MaxPoints < 0 || o.MaxPoints > 4000 {
		return fmt.Errorf("history points must be within 1..4000")
	}
	return nil
}

type Issue struct {
	Code   string `json:"code"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Detail string `json:"detail,omitempty"`
}
type Session struct {
	ID           string    `json:"id"`
	Source       string    `json:"source"`
	StartedAt    time.Time `json:"started_at"`
	Parts        int       `json:"parts"`
	Closed       bool      `json:"closed"`
	RetainedOnly bool      `json:"retained_only"`
}
