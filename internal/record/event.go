// Package record writes private bounded telemetry event streams, independently of transport.
package record

import "time"

type Event struct {
	Version   int       `json:"schema_version"`
	Sequence  uint64    `json:"sequence"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	ElapsedNS int64     `json:"elapsed_ns"`
	Segment   int       `json:"segment"`
	Data      any       `json:"data"`
}
