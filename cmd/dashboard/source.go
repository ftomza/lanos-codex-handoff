package main

import (
	"context"
	"net"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/ble"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/live"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/serial"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

type dashboardDependencies struct {
	listen func(context.Context, string, string) (net.Listener, error)
	live   func(probe.Config) (telemetry.Source, error)
}

func createSource(o dashboardOptions, deps dashboardDependencies) (telemetry.Source, string, error) {
	if o.source == "sim" {
		source, err := sim.New(o.interval)
		return source, telemetry.Simulator, err
	}
	if deps.live == nil {
		deps.live = newLiveSource
	}
	source, err := deps.live(o.probe)
	return source, telemetry.Live, err
}

func newLiveSource(cfg probe.Config) (telemetry.Source, error) {
	return live.New(cfg, func(ctx context.Context) (elm327.Transport, error) {
		if cfg.Transport == "ble" {
			return ble.Open(ctx, ble.Config{Name: cfg.BLEName, Helper: cfg.BLEHelper})
		}
		return serial.Open(ctx, cfg.Port, cfg.Baud)
	})
}
