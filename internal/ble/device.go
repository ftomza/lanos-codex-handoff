// Package ble adapts a native Go BLE child to the common ELM byte transport.
package ble

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type Config struct{ Name, Helper string }

type Device struct {
	mu         sync.Mutex
	buffer     []byte
	err        error
	ready      bool
	remoteDone bool
	closed     bool
	changed    chan struct{}
	sequence   uint64
	ack        chan blewire.Message
	gate       chan struct{}
	cmd        *exec.Cmd
	stdin      *os.File
	exited     chan struct{}
	waitErr    error
	stderr     boundedLog
	closeOnce  sync.Once
	closeErr   error
}

func Open(ctx context.Context, cfg Config) (*Device, error) {
	if strings.TrimSpace(cfg.Name) == "" || len(cfg.Name) > 128 || strings.ContainsFunc(cfg.Name, unicode.IsControl) {
		return nil, fmt.Errorf("explicit BLE name must be nonempty and contain no control characters")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Helper == "" {
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cfg.Helper = filepath.Join(filepath.Dir(executable), "obdprobe-ble")
	}
	args := []string{"--name", cfg.Name}
	if deadline, ok := ctx.Deadline(); ok {
		// Let a cooperative scan time out and clean up before the parent's
		// hard bound. A wedged native call is still terminated by the parent.
		timeout := max(time.Nanosecond, time.Until(deadline)*9/10)
		args = append(args, "--connect-timeout", timeout.String())
	}
	d, err := openCommand(ctx, exec.Command(cfg.Helper, args...))
	if err != nil {
		return nil, fmt.Errorf("open native BLE: %w; build both binaries with make build-obdprobe; check macOS Bluetooth permission", err)
	}
	return d, nil
}

func (d *Device) notifyLocked() {
	close(d.changed)
	d.changed = make(chan struct{})
}

func (d *Device) fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err == nil {
		d.err = err
		d.buffer = nil
		d.notifyLocked()
	}
}

func (d *Device) Read(ctx context.Context, b []byte) (int, error) {
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
		if d.err != nil {
			err := d.err
			d.mu.Unlock()
			return 0, err
		}
		if len(d.buffer) > 0 {
			n := copy(b, d.buffer)
			d.buffer = d.buffer[n:]
			d.mu.Unlock()
			return n, nil
		}
		changed := d.changed
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-changed:
		}
	}
}

func (d *Device) Write(ctx context.Context, b []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(b) == 0 {
		return 0, nil
	}
	if len(b) > 512 {
		return 0, fmt.Errorf("ble write exceeds byte limit")
	}
	select {
	case d.gate <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-d.gate }()
	d.mu.Lock()
	if d.closed || d.err != nil {
		err := errors.Join(os.ErrClosed, d.err)
		d.mu.Unlock()
		return 0, err
	}
	d.sequence++
	id := d.sequence
	ack := make(chan blewire.Message, 1)
	d.ack = ack
	d.mu.Unlock()
	defer func() { d.mu.Lock(); d.ack = nil; d.mu.Unlock() }()
	if err := d.send(ctx, blewire.Message{Type: "write", ID: id, Data: b}); err != nil {
		d.fail(err)
		return 0, err
	}
	for {
		d.mu.Lock()
		changed, err, closed := d.changed, d.err, d.closed
		d.mu.Unlock()
		if closed {
			return 0, os.ErrClosed
		}
		if err != nil {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-changed:
		case result := <-ack:
			if result.Error != "" {
				return 0, fmt.Errorf("gatt write: %s", result.Error)
			}
			return len(b), nil
		}
	}
}
