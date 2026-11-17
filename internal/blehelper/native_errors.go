package blehelper

import (
	"fmt"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

// The pinned darwin backend exposes these timeouts as private/plain errors.
// Match only its exact result at the relevant native call boundary, never a
// substring of an arbitrary IPC, permission or joined cleanup error.
func nativeError(operation string, err error) error {
	if err == nil {
		return nil
	}
	expected := ""
	switch operation {
	case "write":
		expected = "timeout on Write()"
	case "subscribe", "unsubscribe":
		expected = "timeout on EnableNotifications"
	}
	if expected != "" && err.Error() == expected {
		return fmt.Errorf("%s: %w: %s", operation, blewire.ErrTimeout, err)
	}
	return err
}
