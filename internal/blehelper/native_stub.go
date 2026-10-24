//go:build !nativeble || !darwin || !cgo

package blehelper

import (
	"context"
	"fmt"
)

func OpenNative(ctx context.Context, name string, notify func([]byte), lost func(error)) (Connection, error) {
	return nil, fmt.Errorf("native BLE backend requires macOS with CGO; build with make build-obdprobe (-tags=nativeble)")
}
