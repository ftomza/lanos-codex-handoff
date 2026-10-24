package ble

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type boundedLog struct {
	mu   sync.Mutex
	data []byte
}

func (l *boundedLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, p[:min(len(p), 4096-len(l.data))]...)
	return len(p), nil
}
func (l *boundedLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return string(l.data) }

func openCommand(ctx context.Context, cmd *exec.Cmd) (*Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d := &Device{cmd: cmd, changed: make(chan struct{}), gate: make(chan struct{}, 1), exited: make(chan struct{})}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	file, ok := stdin.(*os.File)
	if !ok {
		return nil, errors.Join(fmt.Errorf("stdin is not an OS pipe"), stdin.Close())
	}
	d.stdin = file
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Join(err, stdin.Close())
	}
	cmd.Stderr = &d.stderr
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(err, stdin.Close(), stdout.Close())
	}
	go d.readEvents(stdout)
	for {
		d.mu.Lock()
		ready, err, changed := d.ready, d.err, d.changed
		d.mu.Unlock()
		if err != nil {
			return nil, errors.Join(err, d.Close())
		}
		if ready {
			return d, nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), d.Close())
		}
	}
}

func (d *Device) readEvents(stdout io.ReadCloser) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024), blewire.MaxLine)
	for scanner.Scan() {
		var m blewire.Message
		err := json.Unmarshal(scanner.Bytes(), &m)
		if err == nil {
			err = d.accept(m)
		}
		if err != nil {
			d.fail(fmt.Errorf("ble protocol: %w", err))
			break
		}
	}
	err := scanner.Err()
	if err == nil {
		err = io.EOF
	}
	d.fail(fmt.Errorf("ble event stream: %w", err))
	closeErr := stdout.Close()
	d.waitErr = errors.Join(d.cmd.Wait(), closeErr)
	close(d.exited)
}

func (d *Device) accept(m blewire.Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch m.Type {
	case "ready":
		if d.ready || m.Version != blewire.Version {
			return fmt.Errorf("unexpected readiness/version")
		}
		d.ready = true
	case "data":
		if len(m.Data) == 0 || len(m.Data) > blewire.MaxData || len(d.buffer)+len(m.Data) > blewire.MaxBuffer {
			return fmt.Errorf("notification buffer limit exceeded")
		}
		d.buffer = append(d.buffer, m.Data...)
	case "ack":
		if !d.ready || m.ID == 0 || m.ID > d.sequence {
			return fmt.Errorf("unexpected write acknowledgement")
		}
		if m.ID != d.sequence || d.ack == nil {
			return nil
		} // A timed-out older write cannot complete a newer one.
		select {
		case d.ack <- m:
		default:
			return fmt.Errorf("duplicate write acknowledgement")
		}
	case "error":
		return fmt.Errorf("native BLE: %s", m.Error)
	default:
		return fmt.Errorf("unknown event %q", m.Type)
	}
	d.notifyLocked()
	return nil
}

func (d *Device) send(ctx context.Context, m blewire.Message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := d.stdin.SetWriteDeadline(deadline); err != nil {
		return err
	}
	done := make(chan struct{})
	var cancellationErr error
	stop := context.AfterFunc(ctx, func() { defer close(done); cancellationErr = d.stdin.SetWriteDeadline(time.Now()) })
	n, err := d.stdin.Write(append(data, '\n'))
	if !stop() {
		<-done
	}
	if ctx.Err() != nil {
		return errors.Join(ctx.Err(), err, cancellationErr)
	}
	if err == nil && n != len(data)+1 {
		err = io.ErrShortWrite
	}
	return err
}

func (d *Device) Close() error {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		d.closed = true
		d.notifyLocked()
		d.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		select {
		case <-d.exited:
		default:
			select {
			case d.gate <- struct{}{}:
				d.closeErr = d.send(ctx, blewire.Message{Type: "close"})
				<-d.gate
			case <-ctx.Done():
				d.closeErr = ctx.Err()
			}
		}
		if err := d.stdin.Close(); !errors.Is(err, os.ErrClosed) {
			d.closeErr = errors.Join(d.closeErr, err)
		}
		select {
		case <-d.exited:
		case <-ctx.Done():
			err := d.cmd.Process.Kill()
			if !errors.Is(err, os.ErrProcessDone) {
				d.closeErr = errors.Join(d.closeErr, err)
			}
			<-d.exited
		}
		d.closeErr = errors.Join(d.closeErr, d.waitErr)
		if d.waitErr != nil {
			d.closeErr = fmt.Errorf("native helper exit: %w; %s", d.closeErr, d.stderr.String())
		}
	})
	return d.closeErr
}
