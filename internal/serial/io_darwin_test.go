package serial

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestNonblockingReadCancellationAndCloseWithoutHardware(t *testing.T) {
	t.Parallel()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	fd, err := syscall.Dup(int(reader.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	device := &Device{fd: fd}
	defer device.Close()
	if err := syscall.SetNonblock(fd, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
	defer cancel()
	buffer := make([]byte, 1)
	if _, err := device.Read(ctx, buffer); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("nonblocking cancellation: %v", err)
	}
	if _, err := writer.Write([]byte{'A'}); err != nil {
		t.Fatal(err)
	}
	if n, err := device.Read(t.Context(), buffer); err != nil || n != 1 || buffer[0] != 'A' {
		t.Fatalf("pipe read: %d %v %q", n, err, buffer)
	}
	if err := device.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := device.Read(t.Context(), buffer); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}
