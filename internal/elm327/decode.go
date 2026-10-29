package elm327

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
)

func HexPID(pid byte) string { return fmt.Sprintf("%02X", pid) }

func payloads(r Response, prefix []byte) ([][]byte, error) {
	if r.Status != OK {
		return nil, fmt.Errorf("response status %s", r.Status)
	}
	frames := make([][]byte, 0, len(r.Lines))
	for _, line := range r.Lines {
		frame, err := hex.DecodeString(compact(line))
		if err != nil || !bytes.HasPrefix(frame, prefix) {
			return nil, fmt.Errorf("invalid or unexpected response payload %q", line)
		}
		frames = append(frames, frame[len(prefix):])
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("missing response payload")
	}
	return frames, nil
}

func singlePayload(r Response, prefix []byte) ([]byte, error) {
	frames, err := payloads(r, prefix)
	if err != nil {
		return nil, err
	}
	for _, frame := range frames[1:] {
		if !bytes.Equal(frame, frames[0]) {
			return nil, fmt.Errorf("ambiguous responses from multiple responders")
		}
	}
	return frames[0], nil
}

type Protocol struct {
	Code      string `json:"code"`
	Automatic bool   `json:"automatic"`
	Number    uint8  `json:"number"`
	Name      string `json:"name"`
}

func DecodeProtocol(r Response) (Protocol, error) {
	var p Protocol
	if r.Status != OK || len(r.Lines) != 1 {
		return p, fmt.Errorf("invalid protocol response")
	}
	p.Code = compact(r.Lines[0])
	code := p.Code
	if len(code) == 2 && code[0] == 'A' {
		p.Automatic = true
		code = code[1:]
	}
	n, err := strconv.ParseUint(code, 16, 8)
	if err != nil || len(code) != 1 {
		return p, fmt.Errorf("invalid protocol code %q", p.Code)
	}
	names := []string{"automatic (not identified)", "SAE J1850 PWM", "SAE J1850 VPW", "ISO 9141-2", "ISO 14230-4 KWP2000 (slow init)", "ISO 14230-4 KWP2000 (fast init)", "ISO 15765-4 CAN (11 bit, 500 kbit/s)", "ISO 15765-4 CAN (29 bit, 500 kbit/s)", "ISO 15765-4 CAN (11 bit, 250 kbit/s)", "ISO 15765-4 CAN (29 bit, 250 kbit/s)", "SAE J1939 CAN", "user CAN 1", "user CAN 2"}
	if int(n) >= len(names) {
		return p, fmt.Errorf("unknown protocol code %q", p.Code)
	}
	p.Number, p.Name = uint8(n), names[n]
	return p, nil
}

func DecodeSupported(base byte, r Response) ([]byte, error) {
	if base%32 != 0 {
		return nil, fmt.Errorf("invalid capability range %02X", base)
	}
	data, err := singlePayload(r, []byte{0x41, base})
	if err != nil {
		return nil, err
	}
	if len(data) != 4 {
		return nil, fmt.Errorf("capability bitmap needs four bytes, got %d", len(data))
	}
	result := make([]byte, 0, 32)
	for i := range 32 {
		if data[i/8]&(1<<(7-i%8)) == 0 {
			continue
		}
		pid := int(base) + i + 1
		if pid > 255 {
			return nil, fmt.Errorf("capability bitmap overflows PID FF")
		}
		result = append(result, byte(pid))
	}
	return result, nil
}

// DecodeDTC decodes stored Mode 03 codes only, with no implication about pending
// or permanent codes. Zero pairs are padding; malformed frames never mean empty.
func DecodeDTC(r Response) ([]string, error) {
	return decodeDTCResponse(r, 0x43)
}

// DecodePendingDTC decodes Mode 07 only; it never treats stored codes as pending.
func DecodePendingDTC(r Response) ([]string, error) {
	return decodeDTCResponse(r, 0x47)
}

func decodeDTCResponse(r Response, responseMode byte) ([]string, error) {
	frames, err := payloads(r, []byte{responseMode})
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, 8)
	for _, data := range frames {
		if len(data) == 0 || len(data)%2 != 0 {
			return nil, fmt.Errorf("malformed DTC response length %d", len(data))
		}
		padding := false
		for i := 0; i < len(data); i += 2 {
			a, b := data[i], data[i+1]
			if a == 0 && b == 0 {
				padding = true
				continue
			}
			if padding {
				return nil, fmt.Errorf("nonzero DTC after padding")
			}
			code := fmt.Sprintf("%c%X%X%02X", "PCBU"[a>>6], (a>>4)&3, a&15, b)
			if !slices.Contains(codes, code) {
				codes = append(codes, code)
			}
		}
	}
	return codes, nil
}
