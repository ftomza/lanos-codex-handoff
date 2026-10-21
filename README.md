# Lanos Dashboard

Local, read-only Chevrolet Lanos dashboard. M0–M2 use **simulated data only**.
No hardware compatibility is claimed. The ECU may be Delphi MR-140; its identity
and standard OBD-II PID support remain unconfirmed.

## Development

Requires Go 1.24 or newer. No external Go modules, network, database, frontend
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

## Hardware scope

`cmd/obdprobe` is an explicit unavailable placeholder until M3 and exits with a
message without accessing hardware. Port enumeration, read-only ELM commands,
PID/DTC parsing and live adapter reconnection are **not implemented in M0–M2**.
Their protocol tests and physical verification remain future work. Android 15 Go
tablet performance, Bluetooth Classic/SPP and ECU compatibility remain untested.

## Boundaries

- `internal/telemetry`: source contract and domain snapshots/freshness.
- `internal/sim`: deterministic simulator implementing the source contract.
- `internal/server`: HTTP transport and telemetry distribution.
- `web`: embedded HTML, CSS, JavaScript; no third-party assets.
- `internal/elm327`, `internal/serial`: reserved for M3/M4, no hardware access.
- `cmd/dashboard`: configuration, lifecycle, application composition.
- `cmd/obdprobe`: explicit unavailable placeholder until M3.

Future live sources will use the same telemetry contract. Diagnostics must stay
read-only; DTC clearing and ECU writes are outside MVP. Bluetooth Classic SPP may
not expose a serial device on macOS M3. Electrical K-Line compatibility alone
does not confirm generic OBD-II support.

See [PROJECT.md](docs/PROJECT.md), [PLAN.md](docs/PLAN.md), and
[STATUS.md](docs/STATUS.md) for scope, verification and handoff.
