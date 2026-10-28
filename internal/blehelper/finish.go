package blehelper

import (
	"context"
	"errors"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

func finishConnection(pump *outputPump, connection Connection, err error) error {
	// A loss callback may cancel startup before ready is queued. Keep the
	// actual cause instead of reporting only internal cancellation.
	select {
	case fault := <-pump.fault:
		if err == nil || errors.Is(err, context.Canceled) {
			err = fault
		} else {
			err = errors.Join(err, fault)
		}
	default:
	}
	var closeErr error
	if connection != nil {
		closeErr = connection.Close()
	}
	code := ""
	if closeErr == nil {
		code = transientCode(err)
	}
	err = errors.Join(err, closeErr)
	if err != nil {
		pump.finalError(err, code)
	}
	pumpErr := pump.finish()
	if code != "" && pumpErr == nil {
		// The event carries the expected outcome; successful exit certifies
		// cleanup. Actual process/cleanup failures remain fatal in the parent.
		return nil
	}
	return errors.Join(err, pumpErr)
}

func transientCode(err error) string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		code := ""
		for i, cause := range joined.Unwrap() {
			next := transientCode(cause)
			if next == "" || i > 0 && next != code {
				return ""
			}
			code = next
		}
		return code
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return transientCode(wrapped.Unwrap())
	}
	if errors.Is(err, blewire.ErrDisconnected) {
		return "disconnected"
	}
	if errors.Is(err, blewire.ErrUnavailable) {
		return "unavailable"
	}
	return ""
}
