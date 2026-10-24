package elm327

import "fmt"

type Measurement struct {
	Value    *float64           `json:"value"`
	Unit     string             `json:"unit"`
	MIL      *bool              `json:"mil_on,omitzero"`
	DTCCount *int               `json:"stored_dtc_count,omitzero"`
	Extra    map[string]float64 `json:"extra,omitzero"`
}

type pidInfo struct {
	name, unit string
	size       int
	decode     func([]byte) float64
}

var pidInfos = map[byte]pidInfo{
	1:    {name: "MIL and monitor status", size: 4},
	4:    {"calculated load", "%", 1, percent},
	5:    {"coolant temperature", "degC", 1, temperature},
	6:    {"short fuel trim bank 1", "%", 1, trim},
	7:    {"long fuel trim bank 1", "%", 1, trim},
	8:    {"short fuel trim bank 2", "%", 1, trim},
	9:    {"long fuel trim bank 2", "%", 1, trim},
	0x0B: {"intake manifold pressure", "kPa", 1, first},
	0x0C: {"engine RPM", "rpm", 2, func(b []byte) float64 { return word(b) / 4 }},
	0x0D: {"vehicle speed", "km/h", 1, first},
	0x0E: {"timing advance", "deg", 1, func(b []byte) float64 { return first(b)/2 - 64 }},
	0x0F: {"intake air temperature", "degC", 1, temperature},
	0x10: {"mass air flow", "g/s", 2, func(b []byte) float64 { return word(b) / 100 }},
	0x11: {"throttle position", "%", 1, percent},
	0x13: {name: "oxygen sensor locations", size: 1},
	0x1C: {"OBD standard", "code", 1, first},
	0x1F: {"engine runtime", "s", 2, word},
	0x2F: {"fuel tank level", "%", 1, percent},
	0x42: {"ECU control module voltage", "V", 2, func(b []byte) float64 { return word(b) / 1000 }},
	0x5E: {"engine fuel rate", "L/h", 2, func(b []byte) float64 { return word(b) / 20 }},
}

func first(b []byte) float64       { return float64(b[0]) }
func word(b []byte) float64        { return float64(uint16(b[0])<<8 | uint16(b[1])) }
func percent(b []byte) float64     { return first(b) * 100 / 255 }
func temperature(b []byte) float64 { return first(b) - 40 }
func trim(b []byte) float64        { return first(b)*100/128 - 100 }

func PIDName(pid byte) string {
	if pid%32 == 0 {
		return "supported PID bitmap"
	}
	if pid >= 0x14 && pid <= 0x1B {
		return fmt.Sprintf("oxygen sensor %d", pid-0x13)
	}
	if info, ok := pidInfos[pid]; ok {
		return info.name
	}
	return "PID " + HexPID(pid)
}

func DecodePID(pid byte, r Response) (Measurement, error) {
	var m Measurement
	data, err := singlePayload(r, []byte{0x41, pid})
	if err != nil {
		return m, err
	}
	info, known := pidInfos[pid]
	if pid >= 0x14 && pid <= 0x1B {
		info = pidInfo{name: PIDName(pid), unit: "V", size: 2, decode: func(b []byte) float64 { return first(b) / 200 }}
		known = true
	}
	if !known {
		if len(data) == 0 {
			return m, fmt.Errorf("empty PID %02X payload", pid)
		}
		return m, nil // Supported, raw payload preserved, no invented decoder.
	}
	if len(data) != info.size {
		return m, fmt.Errorf("PID %02X needs %d bytes, got %d", pid, info.size, len(data))
	}
	m.Unit = info.unit
	if info.decode != nil {
		value := info.decode(data)
		m.Value = &value
	}
	if pid == 1 {
		mil := data[0]&0x80 != 0
		count := int(data[0] & 0x7F)
		m.MIL = &mil
		m.DTCCount = &count
	}
	if pid >= 0x14 && pid <= 0x1B && data[1] != 0xFF {
		m.Extra = map[string]float64{"short_fuel_trim_percent": trim(data[1:])}
	}
	return m, nil
}
