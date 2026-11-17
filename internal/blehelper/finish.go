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
	if transientCode(err) != "" && (closeErr == nil || timeoutCleanup(closeErr)) {
		code = transientCode(errors.Join(err, closeErr))
	}
	err = errors.Join(err, closeErr)
	if err != nil {
		pump.finalError(err, code)
	}
	pumpErr := pump.finish()
	if code != "" && pumpErr == nil {
		// The event retains transient operation/notification timeout evidence.
		// The parent must reap this process before retrying; unknown cleanup
		// errors and actual process/output failures remain fatal.
		return nil
	}
	return errors.Join(err, pumpErr)
}

func timeoutCleanup(err error) bool {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		causes := joined.Unwrap()
		for _, cause := range causes {
			if !timeoutCleanup(cause) {
				return false
			}
		}
		return len(causes) > 0
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return timeoutCleanup(wrapped.Unwrap())
	}
	return errors.Is(err, blewire.ErrTimeout)
}

func transientCode(err error) string {
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		code := ""
		for _, cause := range joined.Unwrap() {
			next := transientCode(cause)
			if next == "" {
				return ""
			}
			if code == "" || next == "timeout" {
				code = next
			}
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
	if errors.Is(err, blewire.ErrTimeout) {
		return "timeout"
	}
	return ""
}
