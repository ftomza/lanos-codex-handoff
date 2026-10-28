# Chevrolet Lanos digital dashboard — project brief

## Context
- Vehicle: Chevrolet Lanos 1.5 8V, reported 2007; VIN indicates ZAZ origin and potentially 2008 model year. 16-pin diagnostic connector.
- ECU: likely Delphi MR-140, **not confirmed** by authoritative ECU part label or live identification.
- Check Engine was initially reported on; Issue #1 (2026-10-24) reports it is now off. Preserve trouble codes and never erase them. One Mode 03 response contained no stored DTCs; pending/permanent codes were not queried.
- Adapter: **Vgate iCar Pro BLE 4.0 Dual**, corrected by the owner in [Issue #1 comment](https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184). Classic SPP through `/dev/cu.Android-Vlink` worked on MacBook M3. `ELM327 v2.3` is the reported ATI identity, not a detected product model.
- Development machine: MacBook M3, macOS. Issue #1 observed `/dev/cu.Android-Vlink` and auto-selected protocol A5 (ISO 14230-4 KWP2000 fast init); Bluetooth can disconnect across ignition changes. Future port availability/reconnect are not guaranteed.
- Target device: Teclast P85, 8-inch 1280×800, Android 15 Go, 3 GB physical RAM, Bluetooth 5.4. Bluetooth Classic/SPP support on tablet remains to be verified.
- Transport direction: prioritize BLE GATT for MacBook/Android, retain Serial/SPP as an alternative behind the same context-aware transport interface. The owner confirmed actual BLE services and ECU responses on MacBook M3, including engine-running sessions ([ignition-on evidence](https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6384173048), [engine-running evidence](https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6384193400)). IOS-Vlink: service 000018f0-0000-1000-8000-00805f9b34fb, RX notify 00002af0-0000-1000-8000-00805f9b34fb, TX write 00002af1-0000-1000-8000-00805f9b34fb. Go obdprobe supports native BLE and Serial/SPP with a shared ELM parser; its Go/macOS BLE baseline passed on this vehicle. The native backend requires Go 1.27, CGO and Xcode tools. Android hardware remains untested. Do not infer long-term reliability or simultaneous Classic/BLE operation.

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

## Live dashboard implementation (2026-10-28)

M4 now reuses the native probe Stream through internal/live and the existing
HTTP/SSE interface. Explicit `--source=elm327` feeds speed/RPM/coolant/adapter
supply; sim remains the default. Run `make build-dashboard`, then
`./bin/dashboard --source=elm327 --transport=ble --ble-name=IOS-Vlink`.
One adapter stream serves all browsers, with capability gating, independent
signal TTLs, bounded retries and no simulated fallback. No dependency changes.
Offline tests passed; new physical dashboard acceptance and Android remain
pending. This does not implement the M6 fuel computer. See NEXT_CONNECTION.md.
