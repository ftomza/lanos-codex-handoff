package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/mil"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milstore"
)

type milExportRow struct {
	milstore.ReadEntry
	File string `json:"file"`
	Line int    `json:"line"`
}

type milIncidentExport struct {
	Version    int                  `json:"version"`
	Scope      string               `json:"scope"`
	Source     string               `json:"source"`
	Session    string               `json:"session_id"`
	Incident   string               `json:"incident"`
	Filtered   bool                 `json:"filtered"`
	Rows       []milExportRow       `json:"rows"`
	Issues     []milstore.ReadIssue `json:"issues"`
	Incomplete bool                 `json:"incomplete"`
}

func readMILIncident(ctx context.Context, dir string, options milanalysis.Options) (milIncidentExport, error) {
	out := milIncidentExport{Version: 1, Scope: "incident_export", Session: options.Session, Incident: options.Incident, Filtered: true, Rows: []milExportRow{}, Issues: []milstore.ReadIssue{}}
	found, start, end, sessionStart, sessionEnd, frame := false, false, false, false, false, false
	var window mil.Window
	var firstFrame, lastFrame time.Time
	var encoded milHistoryBuffer
	issues, err := milstore.Read(ctx, dir, milstore.ReadOptions{Session: options.Session}, func(e milstore.ReadEntry) error {
		if e.Incident != options.Incident && e.Kind != "session_start" && e.Kind != "session_end" {
			return nil
		}
		out.Source = e.Source
		if e.Incident == options.Incident {
			found = true
		}
		switch e.Kind {
		case "session_start":
			sessionStart = true
		case "session_end":
			sessionEnd = true
			var footer struct {
				Reason string `json:"reason"`
				Error  string `json:"error"`
			}
			if err := json.Unmarshal(e.Data, &footer); err != nil {
				return err
			}
			if footer.Reason != "closed" || footer.Error != "" {
				out.Incomplete = true
				out.Issues = append(out.Issues, milstore.ReadIssue{Code: "recording_failed", File: e.File, Line: e.Line, Detail: "session recording did not finish successfully"})
			}
		case "incident_start":
			start = true
			if err := json.Unmarshal(e.Data, &window); err != nil {
				return err
			}
		case "incident_extend", "incident_end":
			if e.Kind == "incident_end" {
				end = true
			}
			if err := json.Unmarshal(e.Data, &window); err != nil {
				return err
			}
		case "pre", "post":
			frame = true
			if firstFrame.IsZero() || e.At.Before(firstFrame) {
				firstFrame = e.At
			}
			if e.At.After(lastFrame) {
				lastFrame = e.At
			}
		}
		row := milExportRow{ReadEntry: e, File: e.File, Line: e.Line}
		if err := json.NewEncoder(&encoded).Encode(row); err != nil {
			return err
		}
		out.Rows = append(out.Rows, row)
		return nil
	})
	if err != nil {
		return out, err
	}
	if !found {
		return out, fmt.Errorf("saved MIL incident not found")
	}
	out.Issues = append(out.Issues, issues...)
	if !start || !end || !frame || !sessionStart || !sessionEnd || !window.PreWindowRetained || !window.PostWindowRetained || window.HadGaps || firstFrame.After(window.RequestedPreFrom) || lastFrame.Before(window.PostUntil) {
		out.Incomplete = true
		out.Issues = append(out.Issues, milstore.ReadIssue{Code: "incomplete_context", Detail: "selected incident context or session boundaries are incomplete"})
	}
	out.Incomplete = out.Incomplete || len(out.Issues) > 0
	return out, ctx.Err()
}
