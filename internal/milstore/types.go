// Package milstore provides bounded, private, asynchronous MIL diagnostic storage.
package milstore

import (
	"encoding/json"
	"fmt"
	"time"
)

const Version = 1
const MaxLine = 1 << 20
const queueSize = 256

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
	if c.Dir == "" || c.Source != "live" && c.Source != "simulator" || c.MaxBytes < 2<<20 || c.MaxBytes > 64<<20 || c.MaxFiles < 1 || c.MaxFiles > 32 {
		return fmt.Errorf("invalid MIL recording configuration")
	}
	return nil
}

type Entry struct {
	Kind     string          `json:"kind"`
	At       time.Time       `json:"at"`
	Incident string          `json:"incident,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

type Status struct {
	State              string `json:"state"`
	Error              string `json:"error,omitempty"`
	Rows               uint64 `json:"rows"`
	RetentionTruncated bool   `json:"retention_truncated"`
	Session            string `json:"session_id,omitempty"`
}

type diskEntry struct {
	Version   int    `json:"version"`
	Session   string `json:"session_id"`
	Source    string `json:"source"`
	Sequence  uint64 `json:"sequence"`
	ElapsedNS int64  `json:"elapsed_ns"`
	Entry
}
