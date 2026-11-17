package probe

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

type FreezeReading struct {
	PID         string            `json:"pid"`
	Source      string            `json:"source"`
	Status      string            `json:"status"`
	CapturedAt  time.Time         `json:"captured_at"`
	RawResponse string            `json:"raw_response"`
	Codes       []string          `json:"codes,omitempty"`
	Frames      []elm327.KWPFrame `json:"frames,omitempty"`
	elm327.Measurement
}

type FreezeFrameResult struct {
	Source   string          `json:"source"`
	Frame    int             `json:"frame"`
	Status   string          `json:"status"`
	Bitmap   *Bitmap         `json:"bitmap,omitempty"`
	Readings []FreezeReading `json:"readings"`
}

func (c Config) finiteDiagnostics() bool {
	return c.ReadPending || c.ReadPermanent || c.ReadFreezeFrame || c.Headers
}

func (r *Runner) readExtended(ctx context.Context, s *elm327.Session, report *Report) error {
	if r.config.ReadPermanent {
		if err := r.readDTC(ctx, s, report, "0A", report.PermanentDTCs, elm327.DecodePermanentDTC); err != nil {
			return err
		}
	}
	if !r.config.ReadFreezeFrame {
		return nil
	}
	pids, err := r.readFreezeBitmap(ctx, s, report, report.FreezeFrame)
	if err != nil {
		return err
	}
	for _, pid := range elm327.FreezeFramePIDs() {
		if slices.Contains(pids, pid) {
			if err := r.readFreezePID(ctx, s, report, report.FreezeFrame, pid); err != nil {
				return err
			}
		}
	}
	return nil
}

// Each helper issues exactly one serialized command so automatic diagnostics can
// later interleave them with ordinary telemetry rather than monopolizing a round.
func (r *Runner) readFreezeBitmap(ctx context.Context, s *elm327.Session, report *Report, result *FreezeFrameResult) ([]byte, error) {
	response, err := r.command(ctx, s, report, "020000")
	if err != nil {
		return nil, err
	}
	bitmap := &Bitmap{Command: "020000", CapturedAt: r.now().UTC(), RawResponse: response.Raw, Supported: []string{}, Status: "available"}
	result.Bitmap = bitmap
	pids, decodeErr := elm327.DecodeFreezeSupported(response)
	status := diagnosticResultStatus(response, 2, decodeErr)
	bitmap.Status, result.Status = status, status
	if status != "available" {
		report.unavailable("freeze-frame bitmap: " + status)
		return nil, nil
	}
	for _, pid := range pids {
		bitmap.Supported = append(bitmap.Supported, elm327.HexPID(pid))
	}
	return pids, nil
}

func (r *Runner) readFreezePID(ctx context.Context, s *elm327.Session, report *Report, result *FreezeFrameResult, pid byte) error {
	command, ok := elm327.FreezeFrameCommand(pid)
	if !ok || pid == 0 {
		return fmt.Errorf("unsupported freeze-frame context PID %02X", pid)
	}
	if result.Bitmap == nil || result.Bitmap.Status != "available" || !slices.Contains(result.Bitmap.Supported, elm327.HexPID(pid)) {
		return fmt.Errorf("freeze-frame PID %02X lacks validated support", pid)
	}
	response, err := r.command(ctx, s, report, command)
	if err != nil {
		return err
	}
	reading := FreezeReading{PID: elm327.HexPID(pid), Source: "ecu_mode02", CapturedAt: r.now().UTC(), RawResponse: response.Raw, Frames: response.Frames}
	var decodeErr error
	if pid == 2 {
		reading.Codes, decodeErr = elm327.DecodeFreezeDTC(response)
	} else {
		reading.Measurement, decodeErr = elm327.DecodeFreezePID(pid, response)
	}
	reading.Status = diagnosticResultStatus(response, 2, decodeErr)
	if reading.Status == "available" && pid != 2 && reading.Value == nil {
		reading.Status = "raw_only"
	}
	if reading.Status != "available" && reading.Status != "raw_only" {
		result.Status = "partial"
		report.unavailable("freeze-frame PID " + reading.PID + ": " + reading.Status)
	}
	result.Readings = append(result.Readings, reading)
	return nil
}

func diagnosticResultStatus(response elm327.Response, service byte, decodeErr error) string {
	if status := elm327.DiagnosticStatus(response, service); status != "" {
		return status
	}
	if decodeErr != nil {
		return "invalid_response"
	}
	return "available"
}

func (f *FreezeFrameResult) invalidate() {
	if f == nil {
		return
	}
	if f.Status == "available" || f.Status == "partial" {
		f.Status = "stale"
	}
	for i := range f.Readings {
		v := &f.Readings[i]
		if v.Status == "available" || v.Status == "raw_only" {
			v.Status = "stale"
			v.Measurement = elm327.Measurement{}
			v.Codes = nil
		}
	}
}

// Restore only after a completed finite scan. Timeouts/cancellation must retain
// the existing prompt-recovery rules rather than writing cleanup while unsynced.
func (r *Runner) restoreHeaders(ctx context.Context, s *elm327.Session, report *Report) error {
	response, err := r.command(ctx, s, report, "ATH0")
	if err != nil {
		return err
	}
	if response.Status != elm327.OK || len(response.Lines) != 1 || response.Lines[0] != "OK" {
		return fmt.Errorf("adapter header restoration ATH0: expected OK")
	}
	return nil
}
