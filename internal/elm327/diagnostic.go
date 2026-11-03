package elm327

import (
	"encoding/hex"
	"fmt"
	"strings"
)

type Monitor struct {
	Name       string `json:"name"`
	Supported  bool   `json:"supported"`
	Incomplete bool   `json:"incomplete"`
}

type Readiness struct {
	Ignition string    `json:"ignition"`
	Raw      []byte    `json:"raw_bytes"`
	Monitors []Monitor `json:"monitors"`
}

func decodeReadiness(data []byte) *Readiness {
	r := &Readiness{Ignition: "spark", Raw: append([]byte(nil), data...), Monitors: []Monitor{}}
	for i, name := range []string{"misfire", "fuel system", "comprehensive components"} {
		r.Monitors = append(r.Monitors, Monitor{name, data[1]&(1<<i) != 0, data[1]&(1<<(i+4)) != 0})
	}
	names := []string{"catalyst", "heated catalyst", "evaporative system", "secondary air", "A/C refrigerant", "oxygen sensor", "oxygen sensor heater", "EGR/VVT"}
	if data[1]&8 != 0 {
		r.Ignition = "compression"
		names = []string{"NMHC catalyst", "NOx/SCR", "reserved", "boost pressure", "reserved", "exhaust gas sensor", "PM filter", "EGR/VVT"}
	}
	for i, name := range names {
		if name != "reserved" {
			r.Monitors = append(r.Monitors, Monitor{name, data[2]&(1<<i) != 0, data[3]&(1<<i) != 0})
		}
	}
	return r
}

func DecodePermanentDTC(r Response) ([]string, error) { return decodeDTCResponse(r, 0x4A) }

// FreezeFramePIDs is the finite context-only subset in the first support block.
func FreezeFramePIDs() []byte {
	return []byte{2, 3, 4, 5, 6, 7, 8, 9, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10, 0x11, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1F}
}

func FreezeFrameCommand(pid byte) (string, bool) {
	if pid == 0 {
		return "020000", true
	}
	for _, p := range FreezeFramePIDs() {
		if p == pid {
			return "02" + HexPID(pid) + "00", true
		}
	}
	return "", false
}

func freezePayload(pid byte, r Response) ([]byte, error) {
	return singlePayload(r, []byte{0x42, pid, 0})
}

func DecodeFreezeSupported(r Response) ([]byte, error) {
	data, err := freezePayload(0, r)
	if err != nil {
		return nil, err
	}
	normalized := Response{Status: OK, Lines: []string{"4100" + hex.EncodeToString(data)}}
	return DecodeSupported(0, normalized)
}

func DecodeFreezePID(pid byte, r Response) (Measurement, error) {
	if _, ok := FreezeFrameCommand(pid); !ok || pid == 0 || pid == 2 {
		return Measurement{}, fmt.Errorf("unsupported freeze-frame context PID %02X", pid)
	}
	data, err := freezePayload(pid, r)
	if err != nil {
		return Measurement{}, err
	}
	if pid == 3 && len(data) != 2 {
		return Measurement{}, fmt.Errorf("freeze-frame PID 03 needs 2 bytes, got %d", len(data))
	}
	return DecodePID(pid, Response{Status: OK, Lines: []string{"41" + HexPID(pid) + hex.EncodeToString(data)}})
}

func DecodeFreezeDTC(r Response) ([]string, error) {
	data, err := freezePayload(2, r)
	if err != nil {
		return nil, err
	}
	if len(data) != 2 {
		return nil, fmt.Errorf("freeze-frame cause requires one DTC pair")
	}
	return DecodeDTC(Response{Status: OK, Lines: []string{"43" + hex.EncodeToString(data)}})
}

// DiagnosticStatus distinguishes explicit service rejection from absence of data.
// Empty means the positive payload still needs its strict service decoder.
func DiagnosticStatus(r Response, service byte) string {
	if r.Status == Malformed {
		return "invalid_response"
	}
	if r.Status != OK && r.Status != ECURejected {
		return "temporarily_unavailable"
	}
	if r.Status == OK {
		return ""
	}
	status := "unsupported"
	for _, line := range r.Lines {
		data, err := hex.DecodeString(compact(line))
		if err != nil || len(data) != 3 || data[0] != 0x7F || data[1] != service {
			return "invalid_response"
		}
		if data[2] != 0x11 && data[2] != 0x12 {
			status = "temporarily_unavailable"
		}
	}
	if len(r.Lines) == 0 {
		return "invalid_response"
	}
	return status
}

// KWPFrame records a validated three-byte physical-address header. Extended
// length, functional headers, CAN and ISO9141 layouts are deliberately rejected.
// Checksum is optional because ELM adapters may omit it; if present it is checked.
type KWPFrame struct {
	Format   string  `json:"format"`
	Target   string  `json:"target"`
	Source   string  `json:"source"`
	Payload  string  `json:"payload"`
	Checksum *string `json:"checksum,omitempty"`
}

func NormalizeKWP(r Response) (Response, error) {
	if r.Status != OK && r.Status != ECURejected {
		return r, nil
	}
	lines := make([]string, 0, len(r.Lines))
	frames := make([]KWPFrame, 0, len(r.Lines))
	for _, line := range r.Lines {
		frame, payload, err := decodeKWP(line)
		if err != nil {
			r.Status = Malformed
			return r, err
		}
		lines = append(lines, strings.ToUpper(hex.EncodeToString(payload)))
		frames = append(frames, frame)
	}
	if len(frames) == 0 {
		r.Status = Malformed
		return r, fmt.Errorf("missing KWP frame")
	}
	r.Lines, r.Frames, r.Status = lines, frames, OK
	for _, line := range lines {
		if lineStatus(line) != OK {
			r.Status = lineStatus(line)
		}
	}
	return r, nil
}

func decodeKWP(line string) (KWPFrame, []byte, error) {
	var f KWPFrame
	data, err := hex.DecodeString(compact(line))
	if err != nil || len(data) < 4 {
		return f, nil, fmt.Errorf("malformed KWP header %q", line)
	}
	n := int(data[0] & 0x3F)
	if data[0]&0xC0 != 0x80 || n == 0 || data[1] != 0xF1 || data[2] == 0 || data[2] == 0xF1 || data[2] == 0xFF {
		return f, nil, fmt.Errorf("unsupported KWP header layout %q", line)
	}
	if len(data) != 3+n && len(data) != 4+n {
		return f, nil, fmt.Errorf("KWP declared length mismatch %q", line)
	}
	f = KWPFrame{Format: HexPID(data[0]), Target: HexPID(data[1]), Source: HexPID(data[2]), Payload: strings.ToUpper(hex.EncodeToString(data[3 : 3+n]))}
	if len(data) == 4+n {
		var sum byte
		for _, b := range data[:len(data)-1] {
			sum += b
		}
		if sum != data[len(data)-1] {
			return KWPFrame{}, nil, fmt.Errorf("invalid KWP checksum %q", line)
		}
		checksum := HexPID(sum)
		f.Checksum = &checksum
	}
	return f, data[3 : 3+n], nil
}
