package serial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ListPorts enumerates names only; it never opens or sends bytes to a device.
func ListPorts() ([]string, error) { return listPorts("/dev/cu.*") }

func listPorts(pattern string) ([]string, error) {
	ports, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("enumerate serial ports: %w", err)
	}
	if ports == nil {
		ports = []string{}
	}
	return ports, nil
}

type Device struct {
	mu     sync.Mutex
	fd     int
	closed bool
}

func Open(ctx context.Context, path string, baud int) (*Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(path, "/dev/cu.") || filepath.Clean(path) != path || strings.Contains(strings.TrimPrefix(path, "/dev/cu."), "/") {
		return nil, fmt.Errorf("explicit macOS /dev/cu.* device required")
	}
	if baud != 9600 && baud != 38400 {
		return nil, fmt.Errorf("supported baud rates: 9600 or 38400")
	}
	fd, err := platformOpen(path, baud)
	if err != nil {
		return nil, fmt.Errorf("open serial %s at %d baud: %w", path, baud, err)
	}
	d := &Device{fd: fd}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("serial open canceled: %w", errors.Join(err, d.Close()))
	}
	return d, nil
}

func (d *Device) Read(ctx context.Context, b []byte) (int, error)  { return d.transfer(ctx, b, false) }
func (d *Device) Write(ctx context.Context, b []byte) (int, error) { return d.transfer(ctx, b, true) }

func (d *Device) transfer(ctx context.Context, b []byte, write bool) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		d.mu.Lock()
		if d.closed {
			d.mu.Unlock()
			return 0, os.ErrClosed
		}
		n, err := platformIO(d.fd, b, write)
		d.mu.Unlock()
		if n > 0 {
			return n, err
		}
		if err == nil {
			return 0, io.EOF
		}
		if !retryable(err) {
			return 0, err
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}

func (d *Device) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return platformClose(d.fd)
}
