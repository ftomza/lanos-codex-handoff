package live

import (
	"strings"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/record"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type signalDef struct {
	name, command, unit, source string
	ttl                         time.Duration
}

var definitions = []signalDef{
	{"map_kpa", "010B", "kPa", "ecu_mode01", 3 * time.Second},
	{"iat_c", "010F", "degC", "ecu_mode01", 20 * time.Second},
	{"maf_g_s", "0110", "g/s", "ecu_mode01", 3 * time.Second},
	{"fuel_rate_lph", "015E", "L/h", "ecu_mode01", 3 * time.Second},
	{"rpm", "010C", "rpm", "ecu_mode01", 3 * time.Second},
	{"speed_kmh", "010D", "km/h", "ecu_mode01", 3 * time.Second},
	{"coolant_c", "0105", "degC", "ecu_mode01", 35 * time.Second},
	{"voltage_v", "ATRV", "V", "adapter_atrv", 130 * time.Second},
}

func initialSignals() map[string]telemetry.Signal {
	signals := make(map[string]telemetry.Signal, len(definitions))
	for _, def := range definitions {
		signals[def.name] = telemetry.Signal{Unit: def.unit, Source: def.source, Support: "not_yet_tested", Status: "not_yet_tested", StaleAfterMS: def.ttl.Milliseconds()}
	}
	return signals
}

func (s *Source) consume(event record.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.latest = s.latest.Clone()
	switch event.Type {
	case "segment_start":
		s.latest.Generation++
		s.clear(telemetry.Connecting, "Подключение к ЭБУ", true)
	case "gap":
		s.latest.Diagnostics.Gaps++
		s.clear(telemetry.Disconnected, "Связь потеряна. Выполняется восстановление.", false)
	case "recovery":
		s.latest.Diagnostics.Recoveries++
		s.clear(telemetry.Connecting, "Ожидаем ответ адаптера для восстановления связи", false)
	case "standby":
		if data, ok := event.Data.(probe.Standby); ok {
			s.clear(telemetry.Connecting, standbyMessage(data.Reason), false)
		}
	case "capabilities":
		if data, ok := event.Data.(probe.RecordingCapabilities); ok {
			s.capabilities(data)
		}
	case "command":
		if data, ok := event.Data.(probe.RecordingCommand); ok {
			s.command(data)
		}
	default:
		return nil
	}
	s.latest.RPM = s.latest.Signals["rpm"].Value
	s.latest.SpeedKMH = s.latest.Signals["speed_kmh"].Value
	s.latest.CoolantC = s.latest.Signals["coolant_c"].Value
	s.latest.VoltageV = s.latest.Signals["voltage_v"].Value
	s.publish()
	return nil
}

func (s *Source) clear(health telemetry.Health, message string, reset bool) {
	s.active = false
	s.ecuSeen = false
	s.latest.Health, s.latest.Message = health, message
	s.latest.RPM, s.latest.SpeedKMH, s.latest.CoolantC, s.latest.VoltageV = nil, nil, nil, nil
	s.latest.CapturedAt, s.latest.ReceivedAt = time.Time{}, time.Time{}
	if reset {
		s.latest.Signals = initialSignals()
		s.latest.Adapter, s.latest.Protocol = "", ""
		return
	}
	for key, signal := range s.latest.Signals {
		signal.Value = nil
		if signal.Status == "available" {
			signal.Status = "temporarily_unavailable"
		}
		s.latest.Signals[key] = signal
	}
}

func (s *Source) command(data probe.RecordingCommand) {
	s.latest.Diagnostics.Commands++
	if data.Status != elm327.OK || data.Error != "" || data.Result != nil && (data.Result.Status == "temporarily_unavailable" || data.Result.Status == "invalid_response") {
		s.latest.Diagnostics.Errors++
	}
	ecu := strings.HasPrefix(data.Command, "01") && data.Status == elm327.OK && data.Error == "" &&
		data.Result != nil && (data.Result.Status == "available" || data.Result.Status == "raw_only" || data.Result.Status == "capability")
	if ecu && !data.CompletedAt.IsZero() {
		s.ecuSeen = true
		s.latest.CapturedAt, s.latest.ReceivedAt = data.CompletedAt, s.now().UTC()
	}
	for _, def := range definitions {
		if def.command != data.Command {
			continue
		}
		signal := s.latest.Signals[def.name]
		signal.Value = nil
		signal.Status = "temporarily_unavailable"
		signal.CapturedAt, signal.ReceivedAt = data.CompletedAt, s.now().UTC()
		if data.Result != nil {
			signal.Status = data.Result.Status
			if data.Result.Status == "available" && data.Result.Value != nil {
				signal.Value = new(*data.Result.Value)
				signal.Support = "supported"
			}
		}
		s.latest.Signals[def.name] = signal
	}
	if s.active && ecu {
		s.latest.Health = telemetry.Connected
		s.latest.Message = "Получаем показания ЭБУ"
	}
}

func (s *Source) capabilities(data probe.RecordingCapabilities) {
	s.active = true
	s.latest.Health, s.latest.Message = telemetry.Connecting, standbyMessage("ecu")
	if s.ecuSeen {
		s.latest.Health, s.latest.Message = telemetry.Connected, "Получаем показания ЭБУ"
	}
	s.latest.Adapter = data.Report.Adapter
	if data.Report.Protocol != nil {
		s.latest.Protocol = data.Report.Protocol.Code
	}
	if data.Report.Source != "" {
		s.latest.SourceDetail = data.Report.Source
	}
	for _, def := range definitions {
		if def.command == "ATRV" {
			continue
		}
		for _, capability := range data.Report.Capabilities {
			if capability.PID != def.command[2:] {
				continue
			}
			signal := s.latest.Signals[def.name]
			signal.Support = capability.Support
			if capability.Support != "supported" {
				signal.Value = nil
				signal.Status = capability.Status
			}
			s.latest.Signals[def.name] = signal
		}
	}
}

func standbyMessage(reason string) string {
	switch reason {
	case "ecu":
		return "Адаптер подключён. ЭБУ не отвечает — возможно, зажигание выключено. Ожидаем…"
	case "synchronize":
		return "Ожидаем завершения ответа адаптера для безопасного восстановления связи…"
	default:
		return "Нет связи с адаптером. Ожидаем подключения…"
	}
}
