// Package blewire defines the bounded byte-only protocol to the native BLE child.
package blewire

import "errors"

var (
	ErrDisconnected = errors.New("ble device disconnected")
	ErrUnavailable  = errors.New("ble target unavailable")
)

const (
	Version   = 1
	MaxLine   = 32 * 1024
	MaxData   = 16 * 1024
	MaxBuffer = 64 * 1024
	Service   = "000018f0-0000-1000-8000-00805f9b34fb"
	RX        = "00002af0-0000-1000-8000-00805f9b34fb"
	TX        = "00002af1-0000-1000-8000-00805f9b34fb"
)

type Message struct {
	Type    string `json:"type"`
	Version int    `json:"version,omitempty"`
	ID      uint64 `json:"id,omitempty"`
	Data    []byte `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Code    string `json:"code,omitempty"`
}
