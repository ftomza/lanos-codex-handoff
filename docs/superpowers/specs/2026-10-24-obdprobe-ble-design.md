# BLE support in obdprobe

## Scope and acceptance

Fix the existing read-only Go diagnostic utility, `cmd/obdprobe`, so it can use
the physically verified IOS-Vlink BLE connection. Keep the dashboard and M4 live
Source unchanged in this task. Preserve the Serial/SPP command line and tests.

Acceptance: one `obdprobe` invocation performs BLE discovery/connection,
initialization, capability-gated PID reads, ATRV and stored Mode 03 DTC reading;
prints useful output and exports the existing private JSON report. No simulator
fallback, ECU writes/clears, new Go dependencies or changes to Go version.
Hardware success must be verified with the resulting Go utility separately from
the already successful Python experiments.

## Transport options and recommendation

1. Recommended: Go utility plus an embedded Python BLE bridge using the already
   installed bleak 3.0.2 environment. The bridge handles discovery, GATT writes
   and notification bytes only. All ELM framing, allowlisting, PID/DTC decoding,
   timeouts/recovery, capability gating and reports remain in Go. No Go module
   changes or automatic dependency installation. The tradeoff is an explicit
   Python/bleak runtime requirement for BLE.
2. Native Go BLE library: cleaner runtime packaging, but requires approval of new
   Go dependencies, backend selection and fresh platform/hardware validation.
3. Native CoreBluetooth helper: avoids a third-party Go module, but adds macOS
   native build requirements and substantially more unverified platform code.

This spec describes option 1. Android integration is a later milestone; using
bleak on macOS does not provide an Android native implementation.

## User interface and metadata

`--transport=serial` remains the default. Existing `--port`, `--baud`,
`--list-ports`, timeouts, retry budget and export behavior remain compatible.
BLE requires explicit `--transport=ble`; it never auto-connects from a no-argument
invocation or port enumeration.

New BLE flags: `--ble-name=IOS-Vlink`, `--ble-python=python3`,
`--ble-connect-timeout=30s`. The existing Python environment is selected with
`--ble-python=.venv/bin/python`. BLE rejects serial-only port/explicit baud flags
and `--list-ports`; serial mode rejects explicitly supplied BLE-only options.
Missing Python/bleak, Bluetooth access or adapter produces an actionable error.
There is no automatic pairing, installation or transport fallback.

Example after implementation:

```sh
go build -o bin/obdprobe ./cmd/obdprobe
./bin/obdprobe --transport=ble --ble-name=IOS-Vlink \
  --ble-python=.venv/bin/python --retries=0 --json \
  --output=diagnostics/ble-baseline-001.json
```

BLE reports use `transport=ble_gatt`, `source=elm327_ble` and explicit BLE name/
service/RX/TX context. Serial baud/port must not be presented as BLE settings.
Device model provenance and actual ATI identity remain separate. Human output
also prints adapter voltage, status and units; ATRV is never called generator
terminal voltage or ECU battery voltage. Untested DTCs never imply no codes.

## Boundary and GATT profile

Implement `internal/ble` as the existing `elm327.Transport` Read/Write/Close
interface. Embed its helper so the Go binary does not depend on a working-directory
script. Invoke Python directly with exec arguments, never a shell. Do not modify,
execute or package the user's test.py/test2.py as production implementation.

Observed profile, from Issue #1 and agent physical tests:

- Device name: IOS-Vlink.
- Service: 000018f0-0000-1000-8000-00805f9b34fb.
- RX notify: 00002af0-0000-1000-8000-00805f9b34fb.
- TX write with response: 00002af1-0000-1000-8000-00805f9b34fb.

Validate service membership and notify/write properties. Subscribe before reporting
ready. Use a bounded JSON-lines pipe protocol with base64 notification/write bytes,
ready, numbered write acknowledgements, close and error messages. Preserve bytes
and order across arbitrary notification fragments; do not parse ELM in Python,
flush input, match responses to notification boundaries or drop buffer overflow.
Limit protocol messages and queued data; overflow fails the transport explicitly.
Keep helper diagnostics separate from its stdout protocol.

## Lifecycle and safe recovery

Discovery/connect has its own deadline, separate from 3s ELM command timeouts.
Successful open transfers child-process ownership to Device; canceling the
temporary open context after success must not kill the established connection.
Read/Write honor their passed contexts. Close is idempotent, requests disconnect,
then terminates/reaps an unresponsive child after a bounded grace period. Startup
failure, cancellation, EOF, disconnect and malformed protocol cannot leak a child
or indefinitely block callers. Late acknowledgements cannot satisfy newer writes.

The shared Session enforces its existing 16 KiB response bound and read-only
allowlist. A timeout still requires prompt recovery before another command;
reopening BLE does not silently reset that synchronization requirement. Retries
remain finite. BLE initialization uses ATE0, ATL0, ATI, ATSP0; avoid ATZ because
adapter-reset behavior over BLE has not been physically verified. Keep the serial
initialization sequence unchanged. Discovery and decoding use the existing probe
runner; do not maintain parallel implementations.

## Verification and delivery

Add failing tests first for transport selection/validation, BLE metadata, distinct
open timeout, initialization selection and printed ATRV. Test the process transport
with Go test subprocesses: ready, fragmented/interleaved notifications, write ack
IDs, bounds, EOF/disconnect, cancellation, failed start and idempotent cleanup.
`go test ./...` requires neither Python/bleak nor hardware. Test the Python bridge
separately with stdlib unittest and a fake BLE backend.

Run Go formatting, tests, race, vet, build, `make lint`, existing web tests and
Linux cross-build. Preserve serial behavior and the simulator-only dashboard.
Then run the Go BLE probe on the parked vehicle after confirming its current
ignition/engine state. Save a new private report; document actual results and
remaining limitations in STATUS/capabilities/NEXT_CONNECTION. Commit and push in
the existing feature branch. Do not mark M4 complete.
