package main

import (
	"fmt"
	"io"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func showDiagnosticResults(w io.Writer, r probe.Report) error {
	note := "pending/permanent codes not queried"
	if r.PendingDTCs != nil {
		note = "permanent codes not queried"
	}
	if r.PermanentDTCs != nil {
		note = "pending codes not queried"
		if r.PendingDTCs != nil {
			note = "all requested DTC reads reported separately"
		}
	}
	if _, err := fmt.Fprintf(w, "Stored Mode 03 DTCs: status=%s codes=%v (%s)\n", r.StoredDTCs.Status, r.StoredDTCs.Codes, note); err != nil {
		return err
	}
	for _, v := range []struct {
		name   string
		result *probe.DTCResult
	}{{"Pending Mode 07", r.PendingDTCs}, {"Permanent Mode 0A", r.PermanentDTCs}} {
		if v.result != nil {
			if _, err := fmt.Fprintf(w, "%s DTCs: status=%s codes=%v\n", v.name, v.result.Status, v.result.Codes); err != nil {
				return err
			}
		}
	}
	if r.FreezeFrame != nil {
		if _, err := fmt.Fprintf(w, "Freeze frame %02d: status=%s source=%s (stored context, not current telemetry)\n", r.FreezeFrame.Frame, r.FreezeFrame.Status, r.FreezeFrame.Source); err != nil {
			return err
		}
		for _, v := range r.FreezeFrame.Readings {
			if _, err := fmt.Fprintf(w, "Freeze PID %s: status=%s value=%v unit=%s codes=%v\n", v.PID, v.Status, numberOrUnknown(v.Value), v.Unit, v.Codes); err != nil {
				return err
			}
		}
	}
	for _, c := range r.Capabilities {
		if c.Readiness != nil {
			if _, err := fmt.Fprintf(w, "Readiness ignition=%s raw=%X (supported/incomplete are monitor flags, not a diagnosis)\n", c.Readiness.Ignition, c.Readiness.Raw); err != nil {
				return err
			}
			for _, m := range c.Readiness.Monitors {
				if _, err := fmt.Fprintf(w, "Monitor %s: supported=%t incomplete=%t\n", m.Name, m.Supported, m.Incomplete); err != nil {
					return err
				}
			}
		}
	}
	if unresolvedMIL(r) {
		_, err := fmt.Fprintln(w, "Check Engine requested by ECU: cause is not determined by queried standard diagnostic modes.")
		return err
	}
	return nil
}
func numberOrUnknown(v *float64) any {
	if v == nil {
		return "unknown"
	}
	return *v
}
func unresolvedMIL(r probe.Report) bool {
	on := false
	for _, c := range r.Capabilities {
		on = on || c.Status == "available" && c.MIL != nil && *c.MIL
	}
	if !on {
		return false
	}
	for _, v := range []*probe.DTCResult{&r.StoredDTCs, r.PendingDTCs, r.PermanentDTCs} {
		if v != nil && v.Status == "available" && len(v.Codes) > 0 {
			return false
		}
	}
	if r.FreezeFrame != nil {
		for _, v := range r.FreezeFrame.Readings {
			if v.Status == "available" && len(v.Codes) > 0 {
				return false
			}
		}
	}
	return true
}
