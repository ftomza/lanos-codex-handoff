package probe

import (
	"context"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func (r *Runner) readDTC(ctx context.Context, s *elm327.Session, report *Report, command string, result *DTCResult, decode func(elm327.Response) ([]string, error)) error {
	response, err := r.command(ctx, s, report, command)
	if err != nil {
		return err
	}
	result.CapturedAt, result.RawResponse, result.Frames = r.now().UTC(), response.Raw, response.Frames
	codes, err := decode(response)
	if err != nil {
		result.Status = "temporarily_unavailable"
		if command == "0A" {
			result.Status = diagnosticResultStatus(response, 10, err)
		}
		label := "stored"
		if command == "0A" {
			label = "permanent"
		}
		if command == "07" {
			label = "pending"
		}
		report.unavailable(label + " DTC read unavailable: " + err.Error())
		return nil
	}
	result.Status, result.Codes = "available", codes
	return nil
}
