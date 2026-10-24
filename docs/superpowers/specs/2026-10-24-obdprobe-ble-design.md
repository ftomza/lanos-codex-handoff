# Native Go BLE support in obdprobe

## Scope and decisions

Fix the existing read-only Go diagnostic utility, cmd/obdprobe, so it can use
the physically verified IOS-Vlink BLE connection. Dashboard and M4 live Source
remain separate. Preserve existing Serial/SPP behavior and report protections.

The user chose native Go BLE and explicitly authorized upgrading the project to
Go 1.27. The earlier Python/bleak bridge proposal is withdrawn. No Python runtime
or user diagnostic scripts belong in the production path.

Recommended dependency, pending approval to add it: tinygo.org/x/bluetooth
v0.16.0 and its required transitive dependencies. Pin the tagged release rather
than a moving branch. Its module requires Go 1.25, compatible with the approved
Go 1.27 project minimum. Normal Go builds work; the TinyGo compiler is unnecessary.

## Library comparison and evidence

| Candidate | Assessment |
| --- | --- |
| tinygo.org/x/bluetooth v0.16.0 | Recommended: macOS CoreBluetooth central, notifications and writes with response; recent macOS initialization fix; normal Go support. |
| github.com/go-ble/ble | README explicitly says the macOS portion is not actively maintained; not selected. |
| github.com/tinygo-org/cbgo directly | CoreBluetooth binding, also used by the recommended library; requires implementing more platform callback orchestration ourselves. |

Sources checked on 2026-10-24:

- [Support and macOS build requirements](https://github.com/tinygo-org/bluetooth/tree/v0.16.0).
- [v0.16.0 release](https://github.com/tinygo-org/bluetooth/releases/tag/v0.16.0), including repeated Adapter.Enable fix for macOS.
- [Versioned module dependencies](https://github.com/tinygo-org/bluetooth/blob/v0.16.0/go.mod).
- [Public API](https://pkg.go.dev/tinygo.org/x/bluetooth@v0.16.0).
- [go-ble maintenance notice](https://github.com/go-ble/ble).

The main module is BSD-3-Clause; its macOS cbgo dependency is Apache-2.0. Include
required third-party notices with distributions. No Android support is inferred.
The host already has Go 1.27.2, CGO enabled, Xcode and clang for darwin/arm64.

## Native backend isolation

The selected library constructs CoreBluetooth managers in DefaultAdapter's
package initialization. Importing it directly into obdprobe could therefore touch
Bluetooth during --help, --list-ports, serial use or offline tests. Also, several
library operations use internal timeouts without accepting context.Context.

Isolate it in a small native Go helper executable, obdprobe-ble, started only
after explicit BLE selection. Both executables are ours and built with Go;
ELM framing, allowlisting, decoding, recovery and reports stay in obdprobe.
The helper performs only discovery/GATT operations. A process boundary gives a
hard cleanup limit even if a platform callback stalls. Build the native helper
with a dedicated nativeble build tag on darwin with CGO; ordinary offline tests
use a fake backend without importing/initializing CoreBluetooth.

Implement internal/ble as existing elm327.Transport Read/Write/Close. Invoke the
helper directly, never a shell. Resolve its default path beside obdprobe, with
explicit --ble-helper override and actionable missing-build guidance. Build and
document both artifacts together. Do not auto-install libraries or pair devices.

Use bounded JSON-lines pipes: ready, base64 notification bytes, numbered writes,
write acknowledgements, close and error. Preserve notification bytes and order;
do not parse ELM in the helper, flush responses, drop overflow or interpret each
notification as a complete response. Keep diagnostics off protocol stdout.

## CLI and report

--transport=serial remains the default, with existing port/baud/listing behavior.
No-argument launch and enumeration never start BLE. BLE requires --transport=ble.
New flags: --ble-name=IOS-Vlink, --ble-connect-timeout=30s, --ble-helper=<path>.
Reject conflicting explicit serial/BLE flags; never silently fall back between
transports. A BLE connection deadline is separate from 3s ELM command deadlines.

After implementation:

```sh
make build-obdprobe
./bin/obdprobe --transport=ble --ble-name=IOS-Vlink --retries=0 \
  --json --output=diagnostics/ble-baseline-001.json
```

BLE reports use transport=ble_gatt, source=elm327_ble and explicit device name /
service/RX/TX context. Do not present serial baud or port as BLE settings. Preserve
model provenance, actual ATI identity, VIN redaction and exclusive 0600 output.
Human output prints adapter ATRV, units and availability; it is not called direct
generator or ECU battery voltage. Untested DTC fields never mean no stored codes.

## Observed GATT profile and lifecycle

- Device: IOS-Vlink.
- Service: 000018f0-0000-1000-8000-00805f9b34fb.
- RX notify: 00002af0-0000-1000-8000-00805f9b34fb.
- TX write with response: 00002af1-0000-1000-8000-00805f9b34fb.

Discover the service/characteristics and require successful acknowledged notify
subscription before ready. Use write with response, as physically tested; do not
silently substitute unacknowledged writes. Validate missing channels and reported
operation errors. The public library API does not expose remote characteristic
property bits; do not claim a separate property inspection it cannot perform.

After ready, Device owns the child; cancellation of the temporary open context
must not kill an established connection. Read/Write honor passed contexts. Late
write acknowledgements cannot satisfy new writes. EOF, malformed protocol,
overflow and disconnect fail explicitly. Close is idempotent, requests native
disconnect, then terminates/reaps an unresponsive child after a bounded grace.

Keep shared Session's 16 KiB bound, read-only allowlist, prompt recovery and finite
retry semantics, including across reopen. BLE initialization: ATE0, ATL0, ATI,
ATSP0; no ATZ until its BLE reset behavior is separately verified. Serial
initialization remains unchanged. PID discovery/decoding and Mode 03 reading use
the existing runner, with no separate ELM implementation.

## Acceptance and verification

Add failing tests first for CLI validation, transport metadata, open deadline,
initialization choice and printed ATRV. Test process transport without hardware
using Go test subprocesses: ready, fragmentation, interleaved notifications/acks,
bounds, EOF/disconnect, cancellation, start failure and bounded idempotent cleanup.
Unit-test helper protocol with an injected fake native backend. go test ./...
must require neither hardware, Python nor a separately built helper.

Run formatting, Go tests/race/vet/build, make lint, web tests and Linux cross-build.
Build the native helper separately on macOS with CGO. Upstream support and clean
compilation do not prove operation with this Vgate: confirm current ignition and
engine state before a physical Go probe, save a new private report, and record
actual results/remaining limitations in STATUS and connection instructions.
Commit/push in the existing feature branch. Do not mark M4 complete.
