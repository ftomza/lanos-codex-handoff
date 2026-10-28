# Lanos Dashboard

Local, read-only Chevrolet Lanos dashboard and diagnostic probe. The dashboard
supports explicit **live BLE/serial telemetry** and a separate default simulator.
M4 and M6 implementation are offline-tested; physical dashboard and fuel-accuracy
acceptance are pending. The fuel computer adds local persistent trip estimates,
coverage, reset and reversible calibration.
Issue #1 records successful owner Vgate/KWP2000 tests over SPP and BLE, including
nonzero engine RPM over BLE. Our Go probe now also passed a physical native BLE
baseline on this MacBook/Vgate/Lanos: RPM, coolant, speed, ATRV, MIL and stored
DTC reading. Its SPP attempt received no ATZ response. The exact ECU model
(possibly Delphi MR-140), Android and long-term reliability remain unverified.
An agent-run Python BLE test also completed three engine-running sessions and
60s of continuous RPM/coolant polling without observed failures; this does not
establish Android or long-duration reliability.
See [capabilities](docs/capabilities.md).

The owner corrected the device model to **Vgate iCar Pro BLE 4.0 Dual** in an
[Issue #1 comment](https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184).
Classic SPP through `/dev/cu.Android-Vlink` has worked on MacBook M3. The probe
supports native Go BLE GATT and Serial/SPP, using the same ELM parser. Observed
IOS-Vlink UUIDs and physical results are in capabilities.md. Android and
simultaneous Classic/BLE links remain unverified.

## Development

Requires Go 1.27 or newer. Native BLE uses `tinygo.org/x/bluetooth v0.16.0` and
its dependencies; macOS native builds require CGO and Xcode tools. Python and
the TinyGo compiler are unnecessary. After downloading Go modules, offline
tests and the simulator need no network, database, frontend build system or cloud.

```sh
go run ./cmd/dashboard --source=sim
go test ./...
go vet ./...
go build ./...
make lint
node --test web/model.test.mjs web/feed.test.mjs
```

Open http://127.0.0.1:8080. The command runs without a car or adapter. HTML,
CSS and JavaScript are embedded in the Go binary; run it from any directory.
Node 18+ is only needed for the optional browser logic tests, never for Go tests
or serving the dashboard.

```sh
go build -o bin/dashboard ./cmd/dashboard
./bin/dashboard --source=sim --listen=127.0.0.1:8080
go test -race ./...
```

## Configuration and demo cycle

| Flag | Default | Meaning |
| --- | --- | --- |
| `--source` | `sim` | `sim` or explicit `elm327`; ordinary startup never opens hardware. |
| `--listen` | `127.0.0.1:8080` | Explicitly use `0.0.0.0:8080` to allow trusted LAN clients. |
| `--interval` | `250ms` | Simulator only, at least 50ms; rejected in live mode. |
| `--stale-after` | `3s` | Global silence expiry; in sim must exceed interval. |

For a tablet on the same trusted network, bind to `0.0.0.0:8080` and open
`http://<Mac-LAN-IP>:8080` in its browser. The default stays loopback-only.
LAN mode has no authentication or TLS; restrict it to your trusted local network.
The browser receives HTTP/SSE; only the explicitly selected live backend accesses
the adapter. This does not implement native Android Bluetooth.

Each 60-second cycle idles for 5 seconds, accelerates to 80 km/h through second
25, cruises through 35, decelerates through 45, and idles again. Voltage is
unavailable (JSON `null`, UI `—`) during seconds 24–28. A simulated source
disconnect occurs at seconds 48–54; all measurements are unavailable, followed
by automatic recovery. Coolant warms gradually across cycles. Samples are
deterministic for the same elapsed time. Stop with Ctrl+C or SIGTERM.

In simulator mode the UI explains the demo and labels the source `SIMULATOR`.
Live mode uses `LIVE`; before metadata arrives the label is `UNKNOWN`.
Old readings are hidden when stale or disconnected. Host response-completion and receipt
timestamps are shown in the browser's local timezone (hover for the UTC timestamp).
These are host timestamps, not ECU capture times. These instruments do not
replace the vehicle's factory instruments.

## Live dashboard (M4, physical acceptance pending)

```sh
make build-dashboard
./bin/dashboard --source=elm327 --transport=ble --ble-name=IOS-Vlink
```

Build on macOS with CGO/Xcode tools. Keep `bin/dashboard` and `bin/obdprobe-ble`
together; `--ble-helper` selects another helper path. With `go run`, explicitly
pass `--ble-helper=bin/obdprobe-ble`. Close other OBD clients/recorders first.
Start only with the car parked and its current ignition/engine state confirmed;
follow [NEXT_CONNECTION.md](docs/NEXT_CONNECTION.md) for physical acceptance.
The new dashboard has not yet been tested against the vehicle.

Serial remains explicit: `./bin/dashboard --source=elm327 --transport=serial
--port=/dev/cu.Android-Vlink --baud=38400` (as one command). No implicit fallback.
Only `--source=elm327` allows hardware flags; mixed BLE/serial flags are rejected.
Live transport defaults to BLE, device name to IOS-Vlink. ELM timeouts default
3s command / 30s initial search / 30s prompt recovery; BLE connection deadline
is 30s. Use `--help` for all validated flags. HTTP binds before hardware starts.

One native Source/adapter stream serves all clients through the existing shared
ELM parser, capability discovery, read-only allowlist and serialized schedule.
RPM/speed have 3s TTL, coolant 35s, adapter supply 130s. Another signal never
refreshes those timestamps. Global 3s silence hides all readings even when an
individual TTL is longer. `NO DATA`, stale, unsupported and disconnect are
unavailable values, never zero or simulator fallback. ATRV measures adapter
supply at the OBD connector, not generator terminals or ECU voltage.

The shared recorder schedule targets one RPM/MAP/speed round per second and one
oldest-due supporting item, without catch-up bursts. Actual rate depends on ECU
latency; the existing instruments remain. M6 additionally uses supported fuel-model
inputs; see the fuel computer section below.
Default `--retries=2` gives two additional attempts **over the entire live run**
(maximum 5), with bounded exponential backoff. Timeout/reopen requires the pending
prompt before reinitialization. Terminal failure keeps HTTP available as
LIVE/disconnected with a restart instruction. The browser reconnect button only
retries SSE/server access; restart the dashboard process for a new adapter budget.
Ctrl+C cancels polling, closes/reaps the helper and reports actual cleanup errors.
No in-memory command history or automatic telemetry file is created in live mode;
use the separate recorder for raw evidence, after stopping the dashboard.

## Fuel computer (M6)

Fuel is enabled by default in simulator and live mode. It shows instantaneous
L/h or L/100 km, covered-interval average, estimated liters and since-reset km.
Totals persist separately in ignored `data/fuel/{sim,live}.json` (0600); use
`--fuel-state` for another path and `--fuel=off` to disable the module. Reset and
reversible full-to-full calibration are explicit dashboard controls.

Our Lanos has no advertised MAF/direct fuel flow and its IAT is unverified.
Default live speed-density is unavailable while running. After confirming a
parked session, explicitly opt into a low-confidence estimate:

```sh
./bin/dashboard --source=elm327 --transport=ble --ble-name=IOS-Vlink \
  --fuel-allow-unverified-iat --fuel-state=data/fuel/live-experimental.json
```

No assumed temperature replaces ECU bytes; -40 C and other values outside the
model range are unavailable. Nominal VE/AFR/gasoline density are assumptions.
Gaps never count as zero fuel, unknown travel cannot dilute the covered average,
and restart preserves totals with an incomplete-coverage flag. State/config
errors fail before hardware opens. Checkpoint interval is 5s; actual write errors
are visible. `--fuel-reset-state` explicitly starts a new trip/factor 1; do not
leave it in ordinary startup. Read [FUEL_COMPUTER.md](docs/FUEL_COMPUTER.md) for
models, flags, state/lock recovery, coverage, API and calibration requirements.

## HTTP contract

- `GET /`: dashboard and locally served static assets.
- `GET /api/snapshot`: current snapshot, with freshness recalculated on request.
- `GET /events`: SSE `telemetry` events, latest snapshot immediately on every
  connection, then every 250ms. There is no historical replay or event backlog.

Both API responses contain nullable `rpm`, `speed_kmh`, `coolant_c`, `voltage_v`,
`source`, `captured_at`, `received_at`, `health`, and `message`. The transport adds
`server_time` and `stale_after_ms`. Health is `connecting`, `connected`, `stale`,
or `disconnected`; `connected` describes freshness, not whether a car is present.
The source identifies `simulator` or `live` independently. A missing capture uses Go's
zero timestamp, shown as unavailable in the UI.

When enabled, `fuel` contains the derived view and persistent trip metadata.
Same-origin JSON POST fuel controls change local state only; see FUEL_COMPUTER.md.

Live responses additionally contain `transport`, `source_detail`, adapter ATI
identity, protocol, and `signals` keyed by the four flat field names. Each signal
has value, unit, provenance, support/status, independent host timestamps and TTL.
Flat live numbers are null when expired. Optional `diagnostics` contains run-wide
command/error/gap/recovery-event counters; these survive segment resets without
retaining history. Support is separate from temporary reading availability.

The browser expires data using local elapsed time plus server-provided ages,
so clock differences cannot make old data fresh. Transport loss hides readings
immediately. Reconnect makes up to six attempts after the initial connection,
with delays of 1, 2, 4, 8, 10 and 10 seconds. A silent initial connection times
out after 8 seconds; established connections use at least 8 seconds (or twice
the configured freshness limit). After exhausting retries a touch-friendly
button starts a fresh attempt budget. Successful telemetry resets that budget.

## Read-only diagnostic probe (M3)

```sh
go run ./cmd/obdprobe --help
go run ./cmd/obdprobe --list-ports
```

Enumeration never opens a device. Serial requires an explicit port; BLE requires
explicit `--transport=ble`. Run finite diagnostic probes with the vehicle
parked; start trip recording while parked and leave it unattended until parking
again. For the next physical session, follow
[NEXT_CONNECTION.md](docs/NEXT_CONNECTION.md), including baseline and supported-PID
scan commands, output preservation, error interpretation and reconnect limits.

```sh
make build-obdprobe
mkdir -p diagnostics
./bin/obdprobe --transport=ble --ble-name=IOS-Vlink --retries=0 \
  --json --output=diagnostics/ble-baseline-001.json
```

Keep `bin/obdprobe` and `bin/obdprobe-ble` together. The latter is our native Go
GATT backend, built with `-tags=nativeble`, started only for explicit BLE. This
isolates CoreBluetooth's eager initialization and blocking system calls. Help,
port listing, serial mode and ordinary `go test ./...` never start that backend.
For `go run ./cmd/obdprobe`, add `--ble-helper=bin/obdprobe-ble` after building it.
BLE currently targets macOS with CGO; other builds give clear backend guidance.
See [third-party notices](docs/THIRD_PARTY.md) when distributing native binaries.

| Probe flag | Default | Meaning |
| --- | --- | --- |
| `--transport` | `serial` | `serial` (SPP) or explicit `ble` (native GATT); no automatic fallback. |
| `--ble-name` | `IOS-Vlink` | Exact advertised device name; BLE only. |
| `--ble-connect-timeout` | `30s` | Discovery/connect/subscribe deadline, separate from ELM command timeout. |
| `--ble-helper` | beside CLI | Explicit path to the built native Go backend; BLE only. |
| `--adapter-model` | `Vgate iCar Pro BLE 4.0 Dual` | Owner-reported context, not ATI detection; override for another device or use `--adapter-model=''` for unknown. |
| `--port` | empty | Explicit macOS `/dev/cu.*`; no automatic selection. |
| `--baud` | `38400` | `38400` or `9600`; Issue #1 does not establish the baud. |
| `--timeout` | `3s` | Entire command, including partial write and prompt read; also serial open. |
| `--initial-timeout` | `30s` | Each initial `0100` protocol search. |
| `--recovery-timeout` | `30s` | Wait for a delayed prompt without sending commands. |
| `--retries` | `2` | Additional attempts, maximum 5; backoff 0.5, 1, 2, 4, 4 seconds. Recording uses one total budget for the entire run. |
| `--scan-supported` | false | Follow advertised bitmap continuations and sample declared PIDs. |
| `--json` | false | One JSON report on stdout instead of a human-readable command audit. |
| `--output` | empty | Exclusive-create report JSON or recording JSONL, permissions 0600; recording mode generates a path in diagnostics/ when omitted. |
| `--record` | false | Continuous capability-gated trip recording to private JSONL; start while parked and leave unattended. |
| `--duration` | `4h` | Recording only; positive duration up to 24h. Ctrl+C also finishes cleanly. |
| `--redact-vin` | true | Redact recognizable unsolicited VIN text/Mode 09 VIN lines. VIN is never requested. |

Serial initializes `ATZ`, `ATE0`, `ATL0`, `ATS0`, `ATH0`, `ATI`, `ATSP0`;
BLE initializes `ATE0`, `ATL0`, `ATI`, `ATSP0` without resetting the adapter. It
reads `0100` and `ATDPN`, queries adapter voltage `ATRV`, then RPM/coolant/speed/MIL
only if advertised, and reads stored DTCs with `03`. A full scan follows each
continuation only when bit 32 permits it. Unknown decoder payloads are preserved
as `raw_only`, with no invented numeric value. MAF/fuel level are never assumed.

Each report includes per-command start/end timestamps, raw bytes (CR escaped in
JSON), status/error, response timestamps and per-PID support evidence. `NO DATA`
is temporary unavailability; missing values are null. `status=partial` means the
probe finished but some responses could not be used; it exits 0. A fatal adapter,
transport or synchronization failure sets `failed`, exports the partial audit and
exits nonzero. Ctrl+C also preserves the audit through normal cancellation.

The report separates `device.model` from the original `adapter` ATI identity.
For this device the model's source is `owner_reported`, with `device.evidence_url`
linking to the correction; a different explicit model is `user_supplied`, and an
empty model is `unknown`. Transport/source are `serial_spp`/`elm327_serial` or
`ble_gatt`/`elm327_ble`. BLE reports include name and service/RX/TX context,
without serial port/baud. Notification fragments form a bounded byte stream for
the same ELM framing/parser. Human output prints adapter supply `ATRV`, explicitly
distinct from direct generator terminal or ECU voltage.

Responses are limited to 16 KiB per command. A timeout blocks later writes until
a prompt is consumed, followed by full initialization/discovery. A reopened port
also requires a recovery prompt after a previous incomplete command. If it never
arrives, the probe stops after its retry budget and requires manual intervention;
it neither flushes input blindly nor repairs system Bluetooth pairing.

Real serial access uses native macOS nonblocking raw 8N1 termios, no third-party
Go modules or `stty`. Other platforms can build and run offline tests but cannot
open real ports through this implementation. Native Go BLE passed one physical
baseline on this MacBook/Vgate/car; Android performance/Bluetooth remains unverified.

## Trip recording for fuel-computer research

```sh
make build-obdprobe
caffeinate -i ./bin/obdprobe --transport=ble --record
```

Start while parked; first verify a short parked recording. The file is created
under ignored diagnostics/ and printed on startup. Stop with Ctrl+C after parking.
This collects RPM/MAP/speed plus supporting engine data, raw responses and timing;
it does not calculate fuel consumption. Fuel calibration needs independent
full-to-full liters/odometer notes, not logs alone. Duration defaults to 4h,
file size is capped at 256 MiB and retries are bounded over the entire recording.
Each event is synced before the next command. Gaps and unavailable readings are
explicit; recording mode never grows an in-memory command history.

See [TRIP_RECORDING.md](docs/TRIP_RECORDING.md) for setup, data schedule, failure
behavior, schema and refueling notes. Owner logs from 2026-10-28 cover a 60s
parked recording and a 14m36s drive. IAT validity and failed cleanup after stop
remain unresolved; longer endurance/reconnect are untested. These recorder sessions do not validate the new M4 dashboard or M6 fuel model.

## Boundaries

- `internal/telemetry`: source contract and domain snapshots/freshness.
- `internal/sim`: deterministic simulator implementing the source contract.
- `internal/live`: native read-only stream adapter, independent freshness and bounded state.
- `internal/fuel`: estimates, monotonic trip/coverage, local state and calibration.
- `internal/server`: HTTP transport and telemetry distribution.
- `web`: embedded HTML, CSS, JavaScript; no third-party assets.
- `internal/elm327`: read-only allowlist, framing, recovery, parsers and decoders.
- `internal/serial`: macOS serial and enumeration; no implicit device access.
- `internal/ble`: bounded, cancellable process-backed BLE byte transport.
- `internal/blehelper`: native GATT backend; fake backend in ordinary tests.
- `internal/blewire`: shared byte-event protocol and observed GATT UUIDs.
- `internal/probe`: finite capability discovery, sampling and reconnect/reporting.
- `cmd/dashboard`: configuration, lifecycle, application composition.
- `cmd/obdprobe`: explicit read-only diagnostic CLI and private JSON export.
- `cmd/obdprobe-ble`: isolated native Go/macOS GATT backend executable.

Simulator and live sources use the same telemetry contract. Diagnostics must stay
read-only; DTC clearing and ECU writes are outside MVP. Bluetooth Classic SPP may
not expose a serial device on macOS M3. Electrical K-Line compatibility alone
does not confirm generic OBD-II support.

See [PROJECT.md](docs/PROJECT.md), [PLAN.md](docs/PLAN.md), and
[STATUS.md](docs/STATUS.md) for scope, verification and handoff.
