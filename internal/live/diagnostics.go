package live

import (
	"strings"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/privacy"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func (s *Source) diagnosticContext(data probe.RecordingCommand) {
	status := string(data.Status)
	if data.Result != nil {
		status = data.Result.Status
	}
	s.latest.Poll = &telemetry.Poll{Command: data.Command, Status: status, Error: privacy.VIN(data.Error), LatencyNS: data.LatencyNS, At: data.CompletedAt, Generation: s.latest.Generation}
	if data.Command == "0101" {
		s.updateMIL(data, status)
	}
	diagnostic := data.Command == "0103" || data.Command == "03" || data.Command == "07" || data.Command == "0A" || strings.HasPrefix(data.Command, "02")
	if !diagnostic {
		return
	}
	r := telemetry.ECUReading{Status: status, CapturedAt: data.CompletedAt, ReceivedAt: s.now().UTC(), RawResponse: privacy.VIN(data.RawResponse)}
	if data.Result != nil {
		encoded, err := privacy.JSON(data.Result, true)
		if err != nil {
			r.Status = "invalid_response"
			s.latest.Poll.Error = "encode diagnostic context: " + err.Error()
		} else {
			r.Detail = encoded
		}
	}
	if s.latest.ECUReadings == nil {
		s.latest.ECUReadings = make(map[string]telemetry.ECUReading, 32)
	}
	s.latest.ECUReadings[data.Command] = r
}

func (s *Source) updateMIL(data probe.RecordingCommand, status string) {
	m := &telemetry.MILReading{Status: status, RawResponse: privacy.VIN(data.RawResponse), CapturedAt: data.CompletedAt, ReceivedAt: s.now().UTC(), StaleAfterMS: s.milTTL.Milliseconds()}
	if data.Result != nil && status == "available" && data.Error == "" {
		if data.Result.MIL != nil {
			m.On = new(*data.Result.MIL)
		}
		if data.Result.DTCCount != nil {
			m.DTCCount = new(*data.Result.DTCCount)
		}
		if data.Result.Readiness != nil {
			encoded, err := privacy.JSON(data.Result.Readiness, true)
			if err != nil {
				m.On, m.DTCCount, m.Status = nil, nil, "invalid_response"
				s.latest.Poll.Error = "encode readiness: " + err.Error()
			} else {
				m.Readiness = encoded
			}
		}
	}
	s.latest.MIL = m
}
