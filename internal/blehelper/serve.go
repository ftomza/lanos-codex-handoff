// Package blehelper runs a byte-only native BLE backend in an isolated process.
package blehelper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type Connection interface {
	Write(context.Context, []byte) (int, error)
	Close() error
}

type Opener func(context.Context, string, func([]byte), func(error)) (Connection, error)

func Serve(ctx context.Context, name string, input io.Reader, output io.Writer, open Opener) (err error) {
	runCtx, cancel := context.WithCancel(ctx)
	pump := newPump(cancel, output)
	var connection Connection
	defer func() {
		cancel()
		if connection != nil {
			err = errors.Join(err, connection.Close())
		}
		if err != nil {
			pump.finalError(err)
		}
		err = errors.Join(err, pump.finish())
	}()
	connection, err = open(runCtx, name, pump.notify, pump.fail)
	if err != nil {
		return err
	}
	if connection == nil {
		return fmt.Errorf("native opener returned no connection")
	}
	if err := pump.send(runCtx, blewire.Message{Type: "ready", Version: blewire.Version}); err != nil {
		return err
	}
	requests := readRequests(runCtx, input)
	var sequence uint64
	for {
		select {
		case fault := <-pump.fault:
			return fault
		case <-runCtx.Done():
			select {
			case fault := <-pump.fault:
				return fault
			default:
				return runCtx.Err()
			}
		case request := <-requests:
			if errors.Is(request.err, io.EOF) {
				return nil
			}
			if request.err != nil {
				return request.err
			}
			m := request.message
			if m.Type == "close" && m.ID == 0 && len(m.Data) == 0 {
				return nil
			}
			if m.Type != "write" || m.ID != sequence+1 || len(m.Data) == 0 || len(m.Data) > 512 {
				return fmt.Errorf("invalid or out-of-sequence write request")
			}
			sequence = m.ID
			n, writeErr := connection.Write(runCtx, m.Data)
			if writeErr == nil && n != len(m.Data) {
				writeErr = io.ErrShortWrite
			}
			ack := blewire.Message{Type: "ack", ID: m.ID}
			if writeErr != nil {
				ack.Error = writeErr.Error()
			}
			if err := pump.send(runCtx, ack); err != nil {
				return err
			}
			if writeErr != nil {
				return writeErr
			}
		}
	}
}

// Decode requests with a line bound before JSON/base64 can allocate large data.
func decodeRequest(line []byte) (blewire.Message, error) {
	var m blewire.Message
	err := json.Unmarshal(line, &m)
	return m, err
}
