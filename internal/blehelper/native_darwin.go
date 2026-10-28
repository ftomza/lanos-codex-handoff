//go:build nativeble && darwin && cgo

package blehelper

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type nativeConnection struct {
	device  bluetooth.Device
	rx      bluetooth.DeviceCharacteristic
	tx      bluetooth.DeviceCharacteristic
	closing atomic.Bool
	once    sync.Once
	err     error
}

// The parent process bounds all library calls, including those without contexts.
func OpenNative(ctx context.Context, name string, notify func([]byte), lost func(error)) (connection Connection, err error) {
	adapter := bluetooth.DefaultAdapter
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := adapter.Enable(); err != nil {
		return nil, fmt.Errorf("enable CoreBluetooth: %w", err)
	}
	address, err := scanDevice(ctx, adapter, name)
	if err != nil {
		return nil, err
	}
	device, err := adapter.Connect(address, bluetooth.ConnectionParams{ConnectionTimeout: bluetooth.NewDuration(10 * time.Second)})
	if err != nil {
		return nil, fmt.Errorf("connect BLE: %w", err)
	}
	if device == (bluetooth.Device{}) {
		return nil, fmt.Errorf("native BLE connect returned no peripheral")
	}
	n := &nativeConnection{device: device}
	defer func() {
		if err != nil {
			err = errors.Join(err, n.Close())
		}
	}()
	adapter.SetConnectHandler(func(d bluetooth.Device, connected bool) {
		if !connected && d.Address.String() == device.Address.String() && !n.closing.Load() {
			lost(blewire.ErrDisconnected)
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service, err := bluetooth.ParseUUID(blewire.Service)
	if err != nil {
		return nil, err
	}
	rx, err := bluetooth.ParseUUID(blewire.RX)
	if err != nil {
		return nil, err
	}
	tx, err := bluetooth.ParseUUID(blewire.TX)
	if err != nil {
		return nil, err
	}
	services, err := device.DiscoverServices([]bluetooth.UUID{service})
	if err != nil {
		return nil, fmt.Errorf("discover Vgate service: %w", err)
	}
	if len(services) != 1 {
		return nil, fmt.Errorf("missing Vgate service")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	chars, err := services[0].DiscoverCharacteristics([]bluetooth.UUID{rx, tx})
	if err != nil {
		return nil, fmt.Errorf("discover Vgate channels: %w", err)
	}
	if len(chars) != 2 {
		return nil, fmt.Errorf("missing Vgate notify/write channels")
	}
	n.rx, n.tx = chars[0], chars[1]
	if err := n.rx.EnableNotifications(notify); err != nil {
		return nil, fmt.Errorf("subscribe Vgate RX: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return n, nil
}

func scanDevice(ctx context.Context, adapter *bluetooth.Adapter, name string) (bluetooth.Address, error) {
	addresses := make(chan bluetooth.Address, 1)
	finished := make(chan error, 1)
	go func() {
		finished <- adapter.Scan(func(a *bluetooth.Adapter, result bluetooth.ScanResult) {
			if result.LocalName() != name {
				return
			}
			select {
			case addresses <- result.Address:
				if err := a.StopScan(); err != nil {
					finished <- err
				}
			default:
			}
		})
	}()
	select {
	case address := <-addresses:
		if err := <-finished; err != nil {
			return bluetooth.Address{}, err
		}
		return address, nil
	case err := <-finished:
		if err == nil {
			err = blewire.ErrUnavailable
		}
		return bluetooth.Address{}, err
	case <-ctx.Done():
		stopErr := adapter.StopScan()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && stopErr == nil {
			return bluetooth.Address{}, fmt.Errorf("scan target: %w", blewire.ErrUnavailable)
		}
		return bluetooth.Address{}, errors.Join(ctx.Err(), stopErr)
	}
}

func (n *nativeConnection) Write(ctx context.Context, data []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return n.tx.Write(data)
}

func (n *nativeConnection) Close() error {
	n.once.Do(func() {
		n.closing.Store(true)
		if n.rx != (bluetooth.DeviceCharacteristic{}) {
			n.err = n.rx.EnableNotifications(nil)
		}
		n.err = errors.Join(n.err, n.device.Disconnect())
	})
	return n.err
}
