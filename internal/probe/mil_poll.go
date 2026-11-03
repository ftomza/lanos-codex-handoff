package probe

import (
	"context"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

// Opt-in adds a serialized high-priority MIL slot and one ordinary/job slot.
// Separate slots preserve ordinary telemetry even with the minimum 1s interval.
func (s *recording) pollSupport(ctx context.Context, schedule *pollSchedule) error {
	schedule.observeMIL(s.report, s.r.now())
	if command := schedule.milCommand(s.r.now()); command != "" {
		if err := s.check(ctx); err != nil {
			return err
		}
		if err := s.readScheduled(ctx, command); err != nil {
			return err
		}
		schedule.complete(command, s.r.now())
		if s.report.Capabilities[0].Status == "unsupported" {
			schedule.mil.unsupported[command] = true
		}
		schedule.observeMIL(s.report, s.r.now())
	}
	command := schedule.support(s.r.now())
	if command == "" {
		return nil
	}
	if err := s.check(ctx); err != nil {
		return err
	}
	if schedule.isDiagnostic(command) {
		return s.readDiagnosticScheduled(ctx, schedule, command)
	}
	if err := s.readScheduled(ctx, command); err != nil {
		return err
	}
	schedule.complete(command, s.r.now())
	return nil
}

func (s *recording) readDiagnosticScheduled(ctx context.Context, schedule *pollSchedule, command string) error {
	var result *DTCResult
	var decode func(elm327.Response) ([]string, error)
	switch command {
	case "03":
		result, decode = &s.report.StoredDTCs, elm327.DecodeDTC
	case "07":
		if s.report.PendingDTCs == nil {
			s.report.PendingDTCs = &DTCResult{Source: "ecu_mode07"}
		}
		result, decode = s.report.PendingDTCs, elm327.DecodePendingDTC
	case "0A":
		if s.report.PermanentDTCs == nil {
			s.report.PermanentDTCs = &DTCResult{Source: "ecu_mode0a"}
		}
		result, decode = s.report.PermanentDTCs, elm327.DecodePermanentDTC
	}
	if result != nil {
		if err := s.r.readDTC(ctx, s.session, &s.report, command, result, decode); err != nil {
			return err
		}
		schedule.completeDiagnostic(command, result.Status, nil)
		return nil
	}
	if command == "020000" {
		s.report.FreezeFrame = &FreezeFrameResult{Source: "ecu_mode02", Readings: []FreezeReading{}}
		pids, err := s.r.readFreezeBitmap(ctx, s.session, &s.report, s.report.FreezeFrame)
		if err != nil {
			return err
		}
		schedule.completeDiagnostic(command, s.report.FreezeFrame.Status, pids)
		return nil
	}
	if err := s.r.readFreezePID(ctx, s.session, &s.report, s.report.FreezeFrame, itemPID(command[:4])); err != nil {
		return err
	}
	readings := s.report.FreezeFrame.Readings
	schedule.completeDiagnostic(command, readings[len(readings)-1].Status, nil)
	return nil
}
