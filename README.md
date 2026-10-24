# Lanos Dashboard

Local, read-only Chevrolet Lanos dashboard and M3 diagnostic probe. The dashboard
still uses **simulated data only**; live dashboard integration belongs to M4.
Issue #1 records successful owner Vgate/KWP2000 tests over SPP and BLE, including
nonzero engine RPM over BLE. The Go probe passed offline tests but received no
ATZ response during its physical SPP attempt. Successful Go communication and
the exact ECU model (possibly Delphi MR-140) remain unverified.
An agent-run Python BLE test also completed three engine-running sessions and
60s of continuous RPM/coolant polling without observed failures; this does not
verify a Go BLE implementation, Android or long-duration reliability.
See [capabilities](docs/capabilities.md).

The owner corrected the device model to **Vgate iCar Pro BLE 4.0 Dual** in an
[Issue #1 comment](https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184).
Classic SPP through `/dev/cu.Android-Vlink` has worked on MacBook M3. The current
probe uses Serial/SPP. New owner comments confirm IOS-Vlink GATT channels and
real ECU readings; BLE is now the priority for MacBook/Android, with SPP retained
as an alternative. BLE implementation is still pending; actual UUIDs and evidence
are in capabilities.md. Simultaneous Classic/BLE links remain unverified.

## Development

Requires Go 1.27 or newer. No external Go modules, network, database, frontend
build system, or cloud are required.

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
| `--source` | `sim` | Only simulator is accepted in M0–M2. |
| `--listen` | `127.0.0.1:8080` | Explicitly use `0.0.0.0:8080` to allow trusted LAN clients. |
| `--interval` | `250ms` | Sample interval, at least 50ms. |
| `--stale-after` | `3s` | Expiry of capture/receipt; must exceed interval. |

For a tablet on the same trusted network, bind to `0.0.0.0:8080` and open
`http://<Mac-LAN-IP>:8080` in its browser. The default stays loopback-only.
LAN mode has no authentication or TLS; restrict it to your trusted local network.
No direct Bluetooth access is attempted by the browser or server.

Each 60-second cycle idles for 5 seconds, accelerates to 80 km/h through second
25, cruises through 35, decelerates through 45, and idles again. Voltage is
unavailable (JSON `null`, UI `—`) during seconds 24–28. A simulated source
disconnect occurs at seconds 48–54; all measurements are unavailable, followed
by automatic recovery. Coolant warms gradually across cycles. Samples are
deterministic for the same elapsed time. Stop with Ctrl+C or SIGTERM.

The UI permanently explains the demo mode and labels the source `SIMULATOR`.
Old readings are hidden when stale or disconnected. Both capture and receipt
timestamps are shown in the browser's local timezone (hover for the UTC timestamp).
These instruments do not replace the vehicle's factory instruments.

## HTTP contract

- `GET /`: dashboard and locally served static assets.
- `GET /api/snapshot`: current snapshot, with freshness recalculated on request.
- `GET /events`: SSE `telemetry` events, latest snapshot immediately on every
  connection, then every 250ms. There is no historical replay or event backlog.

Both API responses contain nullable `rpm`, `speed_kmh`, `coolant_c`, `voltage_v`,
`source`, `captured_at`, `received_at`, `health`, and `message`. The transport adds
`server_time` and `stale_after_ms`. Health is `connecting`, `connected`, `stale`,
or `disconnected`; `connected` describes freshness, not whether a car is present.
The source identifies the simulator independently. A missing capture uses Go's
zero timestamp, shown as unavailable in the UI.

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

Enumeration never opens a device. A probe requires an explicit port and should
only be run with the vehicle parked. For the next physical session, follow
[NEXT_CONNECTION.md](docs/NEXT_CONNECTION.md), including baseline and supported-PID
scan commands, output preservation, error interpretation and reconnect limits.

| Probe flag | Default | Meaning |
| --- | --- | --- |
| `--adapter-model` | `Vgate iCar Pro BLE 4.0 Dual` | Owner-reported context, not ATI detection; override for another device or use `--adapter-model=''` for unknown. |
| `--port` | empty | Explicit macOS `/dev/cu.*`; no automatic selection. |
| `--baud` | `38400` | `38400` or `9600`; Issue #1 does not establish the baud. |
| `--timeout` | `3s` | Entire command, including partial write and prompt read; also port open. |
| `--initial-timeout` | `30s` | Each initial `0100` protocol search. |
| `--recovery-timeout` | `30s` | Wait for a delayed prompt without sending commands. |
| `--retries` | `2` | Additional attempts, maximum 5; backoff 0.5, 1, 2, 4, 4 seconds. |
| `--scan-supported` | false | Follow advertised bitmap continuations and sample declared PIDs. |
| `--json` | false | One JSON report on stdout instead of a human-readable command audit. |
| `--output` | empty | Exclusive-create JSON file with permissions 0600; never overwrite logs. |
| `--redact-vin` | true | Redact recognizable unsolicited VIN text/Mode 09 VIN lines. VIN is never requested. |

The baseline initializes `ATZ`, `ATE0`, `ATL0`, `ATS0`, `ATH0`, `ATI`, `ATSP0`,
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
empty model is `unknown`. `transport=serial_spp` describes the current backend,
not BLE capability. The `elm327.Transport` Read/Write/Close boundary remains
independent of serial; the planned GATT adapter must validate the observed
IOS-Vlink channels and turn notifications into a byte stream for the existing
ELM framing/parser, without duplicating protocol logic.

Responses are limited to 16 KiB per command. A timeout blocks later writes until
a prompt is consumed, followed by full initialization/discovery. A reopened port
also requires a recovery prompt after a previous incomplete command. If it never
arrives, the probe stops after its retry budget and requires manual intervention;
it neither flushes input blindly nor repairs system Bluetooth pairing.

Real serial access uses native macOS nonblocking raw 8N1 termios, no third-party
Go modules or `stty`. Other platforms can build and run offline tests but cannot
open real ports through this implementation. Successful Go hardware communication
and Android 15 Go performance/Bluetooth remain unverified.

## Boundaries

- `internal/telemetry`: source contract and domain snapshots/freshness.
- `internal/sim`: deterministic simulator implementing the source contract.
- `internal/server`: HTTP transport and telemetry distribution.
- `web`: embedded HTML, CSS, JavaScript; no third-party assets.
- `internal/elm327`: read-only allowlist, framing, recovery, parsers and decoders.
- `internal/serial`: macOS serial and enumeration; no implicit device access.
- `internal/probe`: finite capability discovery, sampling and reconnect/reporting.
- `cmd/dashboard`: configuration, lifecycle, application composition.
- `cmd/obdprobe`: explicit read-only diagnostic CLI and private JSON export.

Future live sources will use the same telemetry contract. Diagnostics must stay
read-only; DTC clearing and ECU writes are outside MVP. Bluetooth Classic SPP may
not expose a serial device on macOS M3. Electrical K-Line compatibility alone
does not confirm generic OBD-II support.

See [PROJECT.md](docs/PROJECT.md), [PLAN.md](docs/PLAN.md), and
[STATUS.md](docs/STATUS.md) for scope, verification and handoff.
