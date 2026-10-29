package elm327

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const MaxResponseBytes = 16 * 1024

var (
	ErrNotSynchronized  = errors.New("previous command incomplete; prompt recovery required")
	ErrResponseTooLarge = errors.New("ELM response exceeds size limit")
	ErrMalformedFrame   = errors.New("unsolicited bytes after ELM prompt")
)

// Transport operations must honor cancellation, including partially written commands.
type Transport interface {
	Read(context.Context, []byte) (int, error)
	Write(context.Context, []byte) (int, error)
	Close() error
}

type Session struct {
	wire         Transport
	gate         chan struct{}
	synchronized bool // Protected by gate. The owner closes after operations finish.
}

func NewSession(wire Transport) *Session {
	return &Session{wire: wire, gate: make(chan struct{}, 1), synchronized: true}
}

// SafeCommand intentionally excludes arbitrary AT commands, VIN requests and
// every OBD mode except read-only Mode 01, stored-DTC Mode 03 and pending Mode 07.
func SafeCommand(command string) bool {
	switch command {
	case "ATZ", "ATE0", "ATL0", "ATS0", "ATH0", "ATI", "ATSP0", "ATDPN", "ATRV", "03", "07":
		return true
	}
	if len(command) != 4 || !strings.HasPrefix(command, "01") {
		return false
	}
	_, err := hex.DecodeString(command)
	return err == nil && command == strings.ToUpper(command)
}

func (s *Session) Command(ctx context.Context, command string, timeout time.Duration) (Response, error) {
	if !SafeCommand(command) {
		return Response{}, fmt.Errorf("command %q is outside the read-only allowlist", command)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return failed("", err), err
	}
	defer func() { <-s.gate }()
	if !s.synchronized {
		return Response{}, ErrNotSynchronized
	}
	s.synchronized = false
	commandBytes := []byte(command + "\r")
	for len(commandBytes) > 0 {
		n, err := s.wire.Write(ctx, commandBytes)
		if err != nil {
			return failed("", err), err
		}
		if n <= 0 {
			return failed("", io.ErrNoProgress), io.ErrNoProgress
		}
		commandBytes = commandBytes[n:]
	}
	raw, err := s.readFrame(ctx)
	if err != nil {
		return failed(raw, err), err
	}
	s.synchronized = true
	return Parse(command, raw), nil
}

func (s *Session) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Recover only reads. The drained response is returned for audit. Callers must
// initialize and rediscover capabilities after recovery, never reuse old values.
func (s *Session) Recover(ctx context.Context, timeout time.Duration) (Response, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := s.acquire(ctx); err != nil {
		return failed("", err), err
	}
	defer func() { <-s.gate }()
	s.synchronized = false
	raw, err := s.readFrame(ctx)
	if err != nil {
		return failed(raw, err), err
	}
	s.synchronized = true
	return Response{Raw: raw, Status: OK}, nil
}

func (s *Session) readFrame(ctx context.Context) (string, error) {
	raw := make([]byte, 0, 512)
	buf := make([]byte, 256)
	for len(raw) < MaxResponseBytes {
		if err := ctx.Err(); err != nil {
			return string(raw), err
		}
		n, err := s.wire.Read(ctx, buf[:min(len(buf), MaxResponseBytes-len(raw))])
		if n > 0 {
			raw = append(raw, buf[:n]...)
			if i := bytes.IndexByte(raw, '>'); i >= 0 {
				if len(bytes.TrimSpace(raw[i+1:])) != 0 {
					return string(raw), ErrMalformedFrame
				}
				return string(raw), nil
			}
		}
		if err != nil {
			return string(raw), err
		}
		if n == 0 {
			return string(raw), io.ErrNoProgress
		}
	}
	return string(raw), ErrResponseTooLarge
}

func failed(raw string, err error) Response {
	status := TransportError
	if errors.Is(err, context.DeadlineExceeded) {
		status = Timeout
	}
	if errors.Is(err, ErrResponseTooLarge) || errors.Is(err, ErrMalformedFrame) {
		status = Malformed
	}
	return Response{Raw: raw, Status: status}
}

func (s *Session) Close() error { return s.wire.Close() }
