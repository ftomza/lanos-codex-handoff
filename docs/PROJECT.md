# Chevrolet Lanos digital dashboard — project brief

## Context
- Vehicle: Chevrolet Lanos 1.5 8V, reported 2007; VIN indicates ZAZ origin and potentially 2008 model year. 16-pin diagnostic connector.
- ECU: likely Delphi MR-140, **not confirmed** by authoritative ECU part label or live identification.
- Check Engine lamp reportedly remains on: first read and preserve trouble codes; do not erase codes.
- OBD adapter arriving: Vgate iCar Pro Bluetooth 3.0, Bluetooth Classic SPP, ELM327-like AT interface.
- Development machine: MacBook M3, macOS. SPP access is uncertain on modern macOS.
- Target device: Teclast P85, 8-inch 1280×800, Android 15 Go, 3 GB physical RAM, Bluetooth 5.4. Bluetooth Classic/SPP support on tablet remains to be verified.

## Goal
Deliver a local-first read-only vehicle dashboard: speed, RPM, coolant temperature, voltage (if available), connection health, DTC reading, and optional telemetry history. Start on MacBook with simulator; test adapter/ECU when available; deploy to Android later.

## Non-goals for MVP
- Replacing safety-critical factory instruments.
- ECU tuning, programming, immobilizer access, or writing settings.
- Assuming fuel level/oil pressure are available via generic OBD-II.
- Relying on MacBook SPP or ECU PID support without testing.

## Acceptance targets
- `go run ./cmd/dashboard --source=sim` serves a working dashboard locally.
- `go test ./...` passes.
- `go run ./cmd/obdprobe --list-ports` and read-only probe mode give actionable diagnostics.
- Read-only ELM commands are logged with timestamp and raw response; no unsupported PID interpreted as zero.
- UI indicates data source, connectivity, timestamps, stale/disconnected status.
