package elm327

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedTransport struct {
	chunks     [][]byte
	writes     bytes.Buffer
	writeLimit int
}

func (s *scriptedTransport) Read(ctx context.Context, b []byte) (int, error) {
	if len(s.chunks) == 0 {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	n := copy(b, s.chunks[0])
	s.chunks[0] = s.chunks[0][n:]
	if len(s.chunks[0]) == 0 {
		s.chunks = s.chunks[1:]
	}
	return n, nil
}
func (s *scriptedTransport) Write(ctx context.Context, b []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if s.writeLimit > 0 {
		b = b[:min(len(b), s.writeLimit)]
	}
	return s.writes.Write(b)
}
func (s *scriptedTransport) Close() error { return nil }

func TestCommandFragmentationAndPartialWrites(t *testing.T) {
	t.Parallel()
	wire := &scriptedTransport{chunks: [][]byte{[]byte("010C\rSEARCH"), []byte("ING...\r410C0000\r"), []byte(">")}, writeLimit: 2}
	s := NewSession(wire)
	r, err := s.Command(t.Context(), "010C", time.Second)
	if err != nil || r.Status != OK || wire.writes.String() != "010C\r" {
		t.Fatalf("response %+v, error %v, writes %q", r, err, wire.writes.String())
	}
}

func TestTimeoutBlocksNextCommandUntilLatePrompt(t *testing.T) {
	t.Parallel()
	wire := &scriptedTransport{chunks: [][]byte{[]byte("410C0000\r")}}
	s := NewSession(wire)
	r, err := s.Command(t.Context(), "010C", 5*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) || r.Raw != "410C0000\r" || r.Status != Timeout {
		t.Fatalf("partial: %+v %v", r, err)
	}
	if _, err := s.Command(t.Context(), "010D", time.Second); !errors.Is(err, ErrNotSynchronized) {
		t.Fatalf("next command must be blocked: %v", err)
	}
	if wire.writes.String() != "010C\r" {
		t.Fatal("wrote while previous response incomplete")
	}
	wire.chunks = append(wire.chunks, []byte("\r>"))
	recovered, err := s.Recover(t.Context(), time.Second)
	if err != nil || recovered.Raw != "\r>" {
		t.Fatalf("recovery: %+v %v", recovered, err)
	}
	wire.chunks = append(wire.chunks, []byte("ELM327 v2.3\r>"))
	if _, err := s.Command(t.Context(), "ATI", time.Second); err != nil {
		t.Fatal(err)
	}
	if wire.writes.String() != "010C\rATI\r" {
		t.Fatalf("unsafe recovery writes: %q", wire.writes.String())
	}
}

func TestSafetyAndBounds(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"04", "ATWR", "ATSH8110F1", "ATSP5", "0902", "010C\r04", "", "01GG"} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			wire := &scriptedTransport{}
			s := NewSession(wire)
			if _, err := s.Command(t.Context(), command, time.Second); err == nil {
				t.Fatalf("accepted %q", command)
			}
			if wire.writes.Len() != 0 {
				t.Fatal("unsafe command reached wire")
			}
		})
	}
	wire := &scriptedTransport{chunks: [][]byte{[]byte(strings.Repeat("A", MaxResponseBytes+1))}}
	s := NewSession(wire)
	r, err := s.Command(t.Context(), "ATI", time.Second)
	if !errors.Is(err, ErrResponseTooLarge) || len(r.Raw) > MaxResponseBytes {
		t.Fatalf("unbounded: %d, %v", len(r.Raw), err)
	}
	if _, err := s.Command(t.Context(), "ATZ", time.Second); !errors.Is(err, ErrNotSynchronized) {
		t.Fatal("overflow session remained usable")
	}
}

func TestUnexpectedTrailingResponse(t *testing.T) {
	t.Parallel()
	wire := &scriptedTransport{chunks: [][]byte{[]byte("410C0000\r>410D10\r>")}}
	s := NewSession(wire)
	if response, err := s.Command(t.Context(), "010C", time.Second); err == nil || response.Status != Malformed {
		t.Fatalf("unsolicited trailing bytes must be a malformed frame, not a disconnect: %+v %v", response, err)
	}
	if _, err := s.Command(t.Context(), "010D", time.Second); !errors.Is(err, ErrNotSynchronized) {
		t.Fatal("trailing payload could be misattributed")
	}
}
