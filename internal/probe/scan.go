package probe

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func (r *Runner) scan(ctx context.Context, s *elm327.Session, report *Report) error {
	if err := r.initialize(ctx, s, report); err != nil {
		return err
	}
	if err := r.discover(ctx, s, report); err != nil {
		return err
	}
	response, err := r.command(ctx, s, report, "ATDPN")
	if err != nil {
		return err
	}
	protocol, err := elm327.DecodeProtocol(response)
	if err != nil {
		report.unavailable("protocol: " + err.Error())
	} else {
		report.Protocol = &protocol
	}
	if err := r.voltage(ctx, s, report); err != nil {
		return err
	}
	for _, pid := range r.sampleOrder(report) {
		if err := r.readPID(ctx, s, report, pid); err != nil {
			return err
		}
	}
	response, err = r.command(ctx, s, report, "03")
	if err != nil {
		return err
	}
	report.StoredDTCs.CapturedAt = r.now().UTC()
	report.StoredDTCs.RawResponse = response.Raw
	codes, err := elm327.DecodeDTC(response)
	if err != nil {
		report.StoredDTCs.Status = "temporarily_unavailable"
		report.unavailable("stored DTC read unavailable: " + err.Error())
		return nil
	}
	report.StoredDTCs.Status = "available"
	report.StoredDTCs.Codes = codes
	return nil
}

func (r *Runner) initialize(ctx context.Context, s *elm327.Session, report *Report) error {
	commands := []string{"ATZ", "ATE0", "ATL0", "ATS0", "ATH0", "ATI", "ATSP0"}
	if r.config.Transport == "ble" {
		commands = []string{"ATE0", "ATL0", "ATI", "ATSP0"}
	}
	for _, command := range commands {
		response, err := r.command(ctx, s, report, command)
		if err != nil {
			return err
		}
		if response.Status != elm327.OK {
			return fmt.Errorf("adapter initialization %s: %s", command, response.Status)
		}
		identity := strings.Join(response.Lines, " ")
		if command == "ATZ" || command == "ATI" {
			upper := strings.ToUpper(identity)
			if !strings.Contains(upper, "ELM") && !strings.Contains(upper, "STN") {
				return fmt.Errorf("adapter initialization %s: unexpected identity", command)
			}
			if command == "ATI" {
				report.Adapter = identity
			}
			continue
		}
		if len(response.Lines) != 1 || response.Lines[0] != "OK" {
			return fmt.Errorf("adapter initialization %s: expected OK", command)
		}
	}
	return nil
}

func (r *Runner) discover(ctx context.Context, s *elm327.Session, report *Report) error {
	for base := 0; base <= 0xE0; base += 32 {
		command := "01" + elm327.HexPID(byte(base))
		response, err := r.command(ctx, s, report, command)
		if err != nil {
			return err
		}
		bitmap := Bitmap{Command: command, CapturedAt: r.now().UTC(), RawResponse: response.Raw, Supported: []string{}, Status: "available"}
		pids, err := elm327.DecodeSupported(byte(base), response)
		if err != nil {
			bitmap.Status = "temporarily_unavailable"
			report.Bitmaps = append(report.Bitmaps, bitmap)
			if base > 0 {
				c := &report.Capabilities[base-1]
				c.Status = "temporarily_unavailable"
				c.RawResponse = response.Raw
				c.CapturedAt = bitmap.CapturedAt
			}
			report.unavailable(command + ": capability response unavailable: " + err.Error())
			return nil
		}
		for _, pid := range pids {
			bitmap.Supported = append(bitmap.Supported, elm327.HexPID(pid))
		}
		report.Bitmaps = append(report.Bitmaps, bitmap)
		for pid := base + 1; pid <= min(base+32, 255); pid++ {
			c := &report.Capabilities[pid-1]
			c.Support = "unsupported"
			c.Status = "unsupported"
			if slices.Contains(pids, byte(pid)) {
				c.Support = "supported"
				c.Status = "not_yet_tested"
			}
			c.SupportEvidence = command
			c.SupportCapturedAt = bitmap.CapturedAt
		}
		if base > 0 {
			c := &report.Capabilities[base-1]
			c.Status = "capability"
			c.RawResponse = response.Raw
			c.CapturedAt = bitmap.CapturedAt
		}
		if !r.config.ScanSupported || base == 0xE0 || !slices.Contains(pids, byte(base+32)) {
			return nil
		}
	}
	return nil
}

func (r *Runner) sampleOrder(report *Report) []byte {
	order := []byte{0x0C, 5, 0x0D, 1}
	if r.config.ScanSupported {
		order = []byte{0x0C, 0x0D, 5, 0x0B, 0x0F, 0x11, 4, 6, 7, 8, 9, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x10, 0x5E, 1}
		for pid := 1; pid <= 255; pid++ {
			if pid%32 != 0 && !slices.Contains(order, byte(pid)) {
				order = append(order, byte(pid))
			}
		}
	}
	return slices.DeleteFunc(order, func(pid byte) bool { return report.Capabilities[int(pid)-1].Support != "supported" })
}

func (r *Runner) readPID(ctx context.Context, s *elm327.Session, report *Report, pid byte) error {
	c := &report.Capabilities[int(pid)-1]
	c.Measurement = elm327.Measurement{}
	c.Status = "temporarily_unavailable"
	response, err := r.command(ctx, s, report, "01"+elm327.HexPID(pid))
	if err != nil {
		return err
	}
	c.RawResponse = response.Raw
	c.CapturedAt = r.now().UTC()
	if response.Status != elm327.OK {
		c.Status = "temporarily_unavailable"
		report.unavailable("PID " + c.PID + ": " + string(response.Status))
		return nil
	}
	measurement, err := elm327.DecodePID(pid, response)
	if err != nil {
		c.Status = "invalid_response"
		report.unavailable("PID " + c.PID + ": " + err.Error())
		return nil
	}
	c.Measurement = measurement
	c.Status = "available"
	if c.Value == nil && c.MIL == nil {
		c.Status = "raw_only"
	}
	return nil
}

func (r *Runner) voltage(ctx context.Context, s *elm327.Session, report *Report) error {
	v := &report.AdapterVoltage
	v.Value, v.Status = nil, "temporarily_unavailable"
	response, err := r.command(ctx, s, report, "ATRV")
	if err != nil {
		return err
	}
	v.CapturedAt, v.RawResponse = r.now().UTC(), response.Raw
	value, err := decodeVoltage(response)
	if err != nil {
		report.unavailable(err.Error())
		return nil
	}
	v.Value, v.Status = &value, "available"
	return nil
}

func decodeVoltage(response elm327.Response) (float64, error) {
	if response.Status != elm327.OK || len(response.Lines) != 1 {
		return 0, fmt.Errorf("adapter voltage unavailable: %s", response.Status)
	}
	text := strings.ToUpper(strings.TrimSpace(response.Lines[0]))
	if !strings.HasSuffix(text, "V") {
		return 0, fmt.Errorf("adapter voltage has invalid units")
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "V")), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 32 {
		return 0, fmt.Errorf("invalid adapter voltage response")
	}
	return value, nil
}
