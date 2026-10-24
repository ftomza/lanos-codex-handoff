package elm327

import (
	"context"
	"errors"
	"testing"
	"time"
)

type controlledTransport struct {
	writes    chan string
	responses chan []byte
}

func (c *controlledTransport) Write(ctx context.Context, b []byte) (int, error) {
	select {
	case c.writes <- string(b):
		return len(b), nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (c *controlledTransport) Read(ctx context.Context, b []byte) (int, error) {
	select {
	case data := <-c.responses:
		return copy(b, data), nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (c *controlledTransport) Close() error { return nil }

func TestConcurrentCommandCannotWriteBeforePrompt(t *testing.T) {
	t.Parallel()
	wire := &controlledTransport{writes: make(chan string, 2), responses: make(chan []byte, 1)}
	s := NewSession(wire)
	result := make(chan error, 1)
	go func() { _, err := s.Command(t.Context(), "010C", time.Second); result <- err }()
	select {
	case command := <-wire.writes:
		if command != "010C\r" {
			t.Fatal(command)
		}
	case <-time.After(time.Second):
		t.Fatal("first command not written")
	}
	if _, err := s.Command(t.Context(), "010D", 5*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiting command not canceled: %v", err)
	}
	if len(wire.writes) != 0 {
		t.Fatal("next command interleaved before prompt")
	}
	wire.responses <- []byte("410C0000\r>")
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first command did not finish")
	}
}
