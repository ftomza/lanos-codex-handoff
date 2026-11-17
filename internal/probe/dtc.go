package probe

import (
	"context"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

func (r *Runner) readDTC(ctx context.Context, s *elm327.Session, report *Report, command string, result *DTCResult, decode func(elm327.Response) ([]string, error)) error {
	result.Codes = nil
	result.Status = "temporarily_unavailable"
	response, err := r.command(ctx, s, report, command)
	if err != nil {
		return err
	}
	result.CapturedAt, result.RawResponse, result.Frames = r.now().UTC(), response.Raw, response.Frames
	codes, err := decode(response)
	if err != nil {
		service := byte(3)
		if command == "07" {
			service = 7
		}
		if command == "0A" {
			service = 10
		}
		result.Status = diagnosticResultStatus(response, service, err)
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
