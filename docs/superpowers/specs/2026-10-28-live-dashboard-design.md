# Native live telemetry for the dashboard

Status: owner approved on 2026-10-28; implementation and offline verification complete.
Physical dashboard acceptance remains pending.

Follow-up: owner reports a successful first combined M4/M6 LIVE run. The approved
[ECU standby design](2026-10-28-ecu-standby-design.md) supersedes the terminal
total-retry policy below for continuous LIVE only. Physical ignition recovery,
timing and endurance acceptance remain pending.

## Requested outcome and milestone

The owner requested real vehicle data in the existing dashboard, in a new branch,
and called this M6. In docs/PLAN.md live Source integration is M4; M6 is the
fuel computer. Implement the requested live dashboard as M4 without renumbering
the plan or silently adding estimated fuel consumption. Branch: feat/live-dashboard,
based on be86564 with the native BLE recorder and analyzed physical recordings.

Keep the existing four instruments: speed, RPM, coolant and adapter supply.
Mark the source LIVE and show meaningful connecting/stale/disconnected states.
Never replace an unavailable real value with simulator output. Simulator mode
remains an explicit separate source, defaulting to sim so an ordinary launch
does not initialize Bluetooth.

Implementation and offline verification do not authorize a new physical car
session. A final parked hardware check requires fresh vehicle-state confirmation.
Existing recorder recordings validate its specific BLE path, not the new Source
integration, Android or live-dashboard recovery.

## Approaches

1. Recommended: native telemetry.Source in Go, reusing the probe/recorder polling
   engine and BLE/serial transports. One adapter connection feeds the existing
   HTTP/SSE server and all browser clients.
2. Tail a recorder JSONL file: avoids some initial source integration, but adds
   a separate process, file handoff latency and more failure/staleness cases.
3. Implement a second BLE polling stack inside the dashboard: duplicates framing,
   capability discovery and recovery that already have physical evidence/tests.

## CLI and build

Implemented build/launch commands (hardware launch requires current vehicle-state confirmation):

```sh
make build-dashboard
./bin/dashboard --source=elm327 --transport=ble --ble-name=IOS-Vlink
```

The server listens on 127.0.0.1:8080 by default. Existing explicit --listen permits
trusted LAN access from an Android browser; this is Mac-hosted BLE, not native
Android Bluetooth support. Keep the current responsive layout and simulator.

--source accepts sim (default) and elm327. For elm327, transport defaults to ble;
serial requires --transport=serial --port=/dev/cu.<explicit> and configurable
baud 38400/9600. Expose the existing BLE name/helper/connect timeout and ELM
command/initial/recovery timeout plus retries through validated dashboard flags.
Reject explicit hardware flags in simulator mode and mixed BLE/serial options.
Keep --interval for simulator sampling; reject an explicitly supplied --interval
in elm327 mode rather than pretending it controls the ELM poll schedule.
--stale-after remains the global silence threshold, default 3s, with positive
validation. Simulator still requires it to exceed the simulation interval.

make build-dashboard builds bin/dashboard and the existing bin/obdprobe-ble
helper beside it. BLE remains explicit and native Go; no module, dependency,
Python or Go-version change. Help, ordinary tests and simulator do not start
CoreBluetooth. A bind/configuration failure must not open the adapter.

## Shared polling and live Source

Add internal/live implementing telemetry.Source. It adapts typed events from a
common probe polling API, not JSON files or another CLI process. Extract only
the shared streaming lifecycle needed from the recorder. Preserve finite probe
reports and --record format/duration/retry behavior with regression tests.

Optional API diagnostics count commands, command errors, gaps and recovery
events for the whole run; counters survive segment resets without retaining
command history.

The live stream runs until context cancellation or terminal retry failure,
without the recorder's four-hour default or 24-hour recording limit. It keeps
only current capability/readings and bounded metrics, never command history.
Use the existing initialization, command allowlist, prompt framing, discovery,
PID/DTC/ATRV decoding and notification assembly. BLE does not send ATZ.

Reuse the observed schedule: core RPM/MAP/speed rounds target one second, with
at most one oldest-due supporting item after each round. Coolant, voltage and
other supporting inputs have lower priority; no catch-up bursts or overlapping
ELM requests. Only advertised Mode 01 PIDs are queried. Read stored DTCs at
initialization as today, never clear codes or request VIN/proprietary commands.

Source Start establishes its lifecycle and returns without waiting for hardware.
A single worker connects/polls; all browsers share it. Read receives the latest
immutable snapshot, dropping intermediate notifications rather than blocking
ELM polling on slow clients. Close is idempotent, cancels blocked work, joins the
worker and surfaces actual cleanup errors. Do not store context.Context fields;
use cancellation functions/channels as in the existing source contract.

## Freshness and availability

Extend Snapshot with optional per-signal metadata while keeping the existing
nullable rpm/speed_kmh/coolant_c/voltage_v API fields. A measurement carries
support, availability, its own captured_at/received_at, stale_after_ms and source.
Host receipt/completion time must not be presented as an ECU capture timestamp.
Source identity is live with BLE/serial metadata available separately.

Default live per-signal TTLs:

| Signal | TTL |
| --- | --- |
| RPM, speed, MAP | 3s |
| Coolant | 35s |
| Adapter voltage | 130s |
| IAT, if exposed in metadata | 20s, with accuracy explicitly unverified |

The longer coolant/voltage TTLs reflect actual 13s/64s polling intervals from the
owner recording. They do not excuse silent link loss: the existing global 3s
silence threshold still transitions to stale and hides values. Fresh updates of
another PID never refresh a signal's timestamps or TTL.

At the backend and browser watchdog, independently expire each field and clear
its numeric output; do not mutate previously published snapshots. Failed PID
reads clear only that measurement immediately. Unsupported, not-yet-tested,
temporarily-unavailable and stale remain distinguishable in API metadata.
If a disconnect/recovery/segment boundary occurs, clear all current values and
require fresh capability discovery/readings; never revive old values after a
new connection. NO DATA and nil values are not zero. RPM zero is a measured
value and does not automatically stop the source or imply fuel cut.

The current global timestamps remain envelope timing for older consumers. UI
uses per-signal metadata when present, and retains the legacy simulator behavior
for snapshots without it. A fresh field does not make another field fresh.

## Recovery, terminal errors and shutdown

Publish disconnected/connecting state before backoff or recovery. Preserve the
existing read-only synchronization rule: after timeout or reopen, consume the
pending prompt before new initialization and discovery; no blind flush.
Default two additional attempts, maximum five, with existing exponential backoff
0.5/1/2/4/4s. For this initial live integration the budget is total for the live
stream, matching the recorder, rather than silently resetting on one good sample.

When retries are exhausted, keep the HTTP dashboard available with LIVE,
disconnected and an instruction to restart the dashboard process. Do not restart
hardware because a browser reconnects. The existing browser retry button relates
to SSE/server access, not a new adapter retry budget. No new hardware-control
HTTP endpoint or automatic Bluetooth pairing in this milestone.

Keep shutdown bounded through the existing helper lifecycle. The owner recording
ended failed after engine-off/Ctrl+C; its precise cleanup error was not retained.
Do not suppress unknown cleanup errors. Add error detail to lifecycle diagnostics
and test cancellation with the helper process so expected cancellation and an
actual shutdown failure can be distinguished. Preserve existing errors/partial
logs when refactoring; only fix a reproduced shutdown defect within this scope.

## Browser behavior

Keep the existing instrument layout. Render LIVE versus SIMULATOR from actual
source identity. Before source metadata arrives, show an unknown/waiting source
instead of claiming a simulator or a live connection. Update demo notice,
connection text, RPM/footer/driving notes and coolant/voltage descriptions for
the selected source. The simulator's intentional six-second outage explanation
must never appear for a live disconnect.

For live mode label voltage as adapter supply at the OBD connector; do not call
it generator-terminal or ECU battery voltage. Explain unsupported/unavailable/
stale measurements with a dash/status, not a numeric zero. Keep the existing
browser elapsed-time watchdog and per-field expiry during silent SSE periods.
No live fuel estimate, assumed IAT substitution or tank-level display.

## Tests and delivery

Tests first, all offline with fake transports/clocks and saved physical response
fixtures. Cover Source lifecycle, start/read/close cancellation, immutable/latest
snapshots, bounded queues/history, capability gating and serialized commands,
per-field expiry without timestamp refresh by another PID, NO DATA clearing,
timeout/late prompt/reopen ordering, finite retry exhaustion and HTTP staying up,
and helper shutdown failure details. Verify existing --record and finite probe
tests remain compatible.

Go HTTP/SSE integration tests exercise one source for multiple clients and LIVE
data without opening Bluetooth. JavaScript tests cover source labels, independent
expiry, missing values, server silence and live disconnect text. Preserve
responsive layout; no visual redesign is needed for this data integration.

Run gofmt/import review, go test ./... -count=1, go test -race ./..., make lint,
make test-web, make build-dashboard, make build-obdprobe, Linux cross-build,
native-helper vet, --help and git diff --check. Offline smoke checks must not
open a device. Document physical acceptance as pending until it actually passes.

Update README, PROJECT/PLAN/STATUS and connection instructions with the exact
live command and limits. Commit/push feat/live-dashboard using the existing
authorized delivery workflow; exclude owner test.py/test2.py and private logs.
