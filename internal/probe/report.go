// Package probe composes read-only ELM discovery and auditable finite probes.
package probe

import (
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

type CommandLog struct {
	StartedElapsedNS   int64         `json:"started_elapsed_ns,omitzero"`
	CompletedElapsedNS int64         `json:"completed_elapsed_ns,omitzero"`
	LatencyNS          int64         `json:"latency_ns,omitzero"`
	Attempt            int           `json:"attempt"`
	Action             string        `json:"action"`
	Command            string        `json:"command,omitzero"`
	Source             string        `json:"source"`
	StartedAt          time.Time     `json:"started_at"`
	CompletedAt        time.Time     `json:"completed_at"`
	RawResponse        string        `json:"raw_response"`
	Status             elm327.Status `json:"status"`
	Error              string        `json:"error,omitzero"`
}

type Capability struct {
	PID               string    `json:"pid"`
	Name              string    `json:"name"`
	Source            string    `json:"source"`
	Support           string    `json:"support"`
	SupportEvidence   string    `json:"support_evidence,omitzero"`
	SupportCapturedAt time.Time `json:"support_captured_at,omitzero"`
	Status            string    `json:"status"`
	CapturedAt        time.Time `json:"captured_at,omitzero"`
	RawResponse       string    `json:"raw_response"`
	elm327.Measurement
}

type Bitmap struct {
	Command     string    `json:"command"`
	CapturedAt  time.Time `json:"captured_at"`
	Status      string    `json:"status"`
	Supported   []string  `json:"supported"`
	RawResponse string    `json:"raw_response"`
}

type DTCResult struct {
	Source      string    `json:"source"`
	Status      string    `json:"status"`
	Codes       []string  `json:"codes"`
	CapturedAt  time.Time `json:"captured_at,omitzero"`
	RawResponse string    `json:"raw_response"`
}

type AdapterVoltage struct {
	Source      string    `json:"source"`
	Status      string    `json:"status"`
	CapturedAt  time.Time `json:"captured_at"`
	RawResponse string    `json:"raw_response"`
	Value       *float64  `json:"value"`
	Unit        string    `json:"unit"`
}

type Report struct {
	streaming      bool
	BLE            *BLEInfo         `json:"ble,omitzero"`
	Device         DeviceInfo       `json:"device"`
	Transport      string           `json:"transport"`
	Source         string           `json:"source"`
	Status         string           `json:"status"`
	Port           string           `json:"port,omitzero"`
	Baud           int              `json:"baud,omitzero"`
	StartedAt      time.Time        `json:"started_at"`
	CompletedAt    time.Time        `json:"completed_at"`
	Attempts       int              `json:"attempts"`
	Adapter        string           `json:"adapter"`
	Protocol       *elm327.Protocol `json:"protocol"`
	Bitmaps        []Bitmap         `json:"bitmaps"`
	Capabilities   []Capability     `json:"capabilities"`
	AdapterVoltage AdapterVoltage   `json:"adapter_voltage"`
	StoredDTCs     DTCResult        `json:"stored_dtcs"`
	Commands       []CommandLog     `json:"commands"`
	Warnings       []string         `json:"warnings"`
	Error          string           `json:"error,omitzero"`
}

type BLEInfo struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	RX      string `json:"rx_notify"`
	TX      string `json:"tx_write"`
}

func resetReport(report *Report, cfg Config, attempt int) {
	logs, started, streaming := report.Commands, report.StartedAt, report.streaming
	*report = Report{streaming: streaming, Device: deviceInfo(cfg.AdapterModel), Transport: "serial_spp", Source: "elm327_serial", Status: "complete", Port: cfg.Port, Baud: cfg.Baud, StartedAt: started,
		Attempts: attempt, Commands: logs, Capabilities: make([]Capability, 255), Bitmaps: []Bitmap{}, Warnings: []string{},
		StoredDTCs:     DTCResult{Source: "ecu_mode03", Status: "not_yet_tested"},
		AdapterVoltage: AdapterVoltage{Source: "adapter_atrv", Status: "not_yet_tested", Unit: "V"}}
	if cfg.Transport == "ble" {
		report.Transport, report.Source = "ble_gatt", "elm327_ble"
		report.Port, report.Baud = "", 0
		report.BLE = &BLEInfo{Name: cfg.BLEName, Service: blewire.Service, RX: blewire.RX, TX: blewire.TX}
	}
	for i := range report.Capabilities {
		pid := byte(i + 1)
		report.Capabilities[i] = Capability{PID: elm327.HexPID(pid), Name: elm327.PIDName(pid), Source: "ecu_mode01", Support: "not_yet_tested", Status: "not_yet_tested"}
	}
}

func (report *Report) unavailable(message string) {
	report.Status = "partial"
	if !report.streaming {
		report.Warnings = append(report.Warnings, message)
	}
}

func (report *Report) invalidate() {
	for i := range report.Capabilities {
		c := &report.Capabilities[i]
		if c.Status == "available" || c.Status == "raw_only" {
			c.Status = "stale"
			c.Measurement = elm327.Measurement{}
		}
	}
	if report.StoredDTCs.Status == "available" {
		report.StoredDTCs.Status = "stale"
		report.StoredDTCs.Codes = nil
	}
	if report.AdapterVoltage.Status == "available" {
		report.AdapterVoltage.Status = "stale"
		report.AdapterVoltage.Value = nil
	}
}
