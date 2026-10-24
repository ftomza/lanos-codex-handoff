package blehelper

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type outputPump struct {
	events chan blewire.Message
	fault  chan error
	stop   chan struct{}
	done   chan struct{}
	bytes  atomic.Int64
	failed atomic.Bool
	once   sync.Once
	cancel context.CancelFunc
	err    error
}

func newPump(cancel context.CancelFunc, out io.Writer) *outputPump {
	p := &outputPump{events: make(chan blewire.Message, 64), fault: make(chan error, 1), stop: make(chan struct{}), done: make(chan struct{}), cancel: cancel}
	go func() {
		defer close(p.done)
		encoder := json.NewEncoder(out)
		write := func(m blewire.Message) bool {
			p.err = encoder.Encode(m)
			p.bytes.Add(-int64(len(m.Data)))
			if p.err != nil {
				p.fail(p.err)
				return false
			}
			return true
		}
		for {
			select {
			case m := <-p.events:
				if !write(m) {
					return
				}
			case <-p.stop:
				for {
					select {
					case m := <-p.events:
						if !write(m) {
							return
						}
					default:
						return
					}
				}
			}
		}
	}()
	return p
}

func (p *outputPump) fail(err error) {
	if err == nil {
		err = fmt.Errorf("ble disconnected")
	}
	p.once.Do(func() { p.failed.Store(true); p.fault <- err; p.cancel() })
}

func (p *outputPump) notify(data []byte) {
	if len(data) == 0 || p.failed.Load() {
		return
	}
	if len(data) > blewire.MaxData {
		p.fail(fmt.Errorf("notification exceeds size bound"))
		return
	}
	if p.bytes.Add(int64(len(data))) > blewire.MaxBuffer {
		p.bytes.Add(-int64(len(data)))
		p.fail(fmt.Errorf("notification queue limit exceeded"))
		return
	}
	m := blewire.Message{Type: "data", Data: append([]byte(nil), data...)}
	select {
	case p.events <- m:
	default:
		p.bytes.Add(-int64(len(data)))
		p.fail(fmt.Errorf("notification event limit exceeded"))
	}
}

func (p *outputPump) send(ctx context.Context, m blewire.Message) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return p.err
	case p.events <- m:
		return nil
	}
}

func (p *outputPump) finalError(err error) {
	message := err.Error()
	if len(message) > 4096 {
		message = message[:4096]
	}
	select {
	case p.events <- blewire.Message{Type: "error", Error: message}:
	case <-p.done:
	}
}

func (p *outputPump) finish() error { p.failed.Store(true); close(p.stop); <-p.done; return p.err }

type request struct {
	message blewire.Message
	err     error
}

func readRequests(ctx context.Context, input io.Reader) <-chan request {
	requests := make(chan request, 1)
	go func() {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 1024), blewire.MaxLine)
		for scanner.Scan() {
			m, err := decodeRequest(scanner.Bytes())
			select {
			case requests <- request{m, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case requests <- request{err: err}:
		case <-ctx.Done():
		}
	}()
	return requests
}
