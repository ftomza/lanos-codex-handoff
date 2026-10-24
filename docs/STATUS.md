# Session handoff / status

## Current state
M0–M3 complete. The dashboard remains simulator-only. The new Go read-only
obdprobe provides macOS serial, ELM framing/recovery, capability discovery,
PID/DTC decoding, bounded reconnect and private JSON export, verified offline.
Issue #1 records the user's successful physical session on 2026-10-24. A later
physical test of the Go probe opened the port but received no ATZ response;
successful Go adapter/ECU communication remains unverified. Bluetooth dropped
even while a diagnostic helper held the port open without sending commands.

Owner correction: the device is **Vgate iCar Pro BLE 4.0 Dual**. Classic SPP
through /dev/cu.Android-Vlink on MacBook M3 is confirmed in the owner's comment.
New owner comments confirm BLE GATT discovery and real ECU responses on MacBook
M3: three engine-off and three engine-running sessions without re-pairing.
BLE is now the priority direction; current Go code still implements Serial/SPP
only. Android, Go BLE and sustained reliability remain unverified.

## Next action
Use the owner's confirmed IOS-Vlink GATT channels for the next BLE transport
implementation, behind the existing elm327.Transport; retain SPP as an alternative.
The previous request for the Python/SPP environment no longer blocks this path.
Select the macOS BLE backend and obtain approval if new dependencies are needed;
do not change go.mod or claim a BLE CLI already exists. Follow with M4 live
Source, measured polling latency/rate, freshness and bounded reconnect tests.
Before the next physical run, confirm the current engine/ignition state afresh;
the newest owner evidence includes engine-running tests performed separately.
Preserve raw data and obtain 0120, 0101, ATRV and advertised MAP/IAT/trim/O2 readings
only through capability-gated read-only polling. See docs/NEXT_CONNECTION.md.

## Completed milestones
- M0 — module, package boundaries, commands, README and Makefile.
- M1 — nullable telemetry, freshness, cancellable source contract and simulator.
- M2 — local HTTP/SSE transport, embedded responsive UI and bounded reconnect.
- M3 — read-only diagnostic CLI, real-session fixture, discovery and recovery.

## Decisions and risks
- Local-first, read-only diagnostics.
- MacBook M3 is development host; Android Teclast P85 is target.
- Issue #1 observed /dev/cu.Android-Vlink, ELM327 v2.3 identity and A5 (KWP2000
  fast init). Ignition changes can break Bluetooth; future connectivity/recovery
  is not guaranteed. Android SPP is untested; Go serial communication has been
  attempted physically but has not yet received any adapter response.
- ECU identity remains unconfirmed. BE3EB811 and RPM/coolant/speed/Mode 03
  responses are known; higher ranges and other readings remain pending.
- Baud was not recorded in Issue #1; 38400/9600 remain explicit options.
- Owner BLE tests returned A5 / BE3EB811 and real nonzero RPM. This does not
  resolve the SPP failure cause, prove Android compatibility or long-run stability.

## Work log
Codex: append date, milestone, commands, test outcomes, blockers and next steps here.

### 2026-10-21 — M0

- Read AGENTS.md, PROJECT.md, PLAN.md and the initial STATUS.md.
- `.codex/START_HERE.md` is absent; a filename search found no copy. Proceeded
  with the explicitly requested M0–M2 scope and existing project documentation.
- Added Go module (Go 1.24 minimum; local toolchain go1.27.2 darwin/arm64),
  package boundaries, bootstrap commands, Makefile, README and implementation notes.
- Commands: `gofmt -w cmd internal web`, `go test ./...`, `go vet ./...`,
  `go build ./...` — all exit 0. Bootstrap packages have no behavioral tests yet.
- `goimports` is not installed; only stdlib imports are used, grouped and sorted
  with gofmt. No modules or development tools were downloaded.
- No implementation blockers. Probe is deliberately unavailable until M3;
  ELM framing/PID and serial tests belong to M3/M4. No hardware access occurred.
- Next: M1 tests for nullable values, freshness, simulator phases and lifecycle.

### 2026-10-21 — M1

- Added source-independent nullable measurements, capture/receipt timestamps,
  explicit source/health and freshness checks for both timestamps.
- Added deterministic 60-second simulator: idle/acceleration/cruise/deceleration,
  unavailable voltage at seconds 24–28, disconnect at 48–54, automatic recovery.
- Tests were added first; initial `go test ./internal/telemetry ./internal/sim`
  failed on missing domain/simulator symbols. Implemented the contract afterward.
- Commands: `gofmt -w internal`, `go test ./...`,
  `go test -race ./internal/telemetry ./internal/sim`, `go vet ./...`,
  `make lint` — passed (exit 0). Lifecycle tests cover pre-start read, duplicate
  start, canceled Read/Start lifecycle, Close unblocking Read and idempotent Close.
- First test run hit sandbox restrictions on the system Go build cache. Repeated
  with the approved Go execution permissions; checks passed. No code blocker.
- No device access or hardware tests. Next: M2 HTTP/SSE and browser state tests.

### 2026-10-21 — M2

- Implemented shared source reader and bounded-memory latest snapshot,
  `GET /api/snapshot`, `GET /events` SSE, and embedded static assets.
- Added simulator-only CLI configuration, loopback default, explicit LAN binding,
  signal cancellation and graceful HTTP shutdown, including active SSE requests.
- Added responsive Russian-language UI with large speed/RPM, coolant/voltage,
  permanent demo notice, SIMULATOR source, health, capture/receipt times, warnings,
  missing readings, expiry watchdog and manual retry after six automatic retries.
- Added HTTP/SSE and JavaScript tests before implementation; initial runs failed
  on missing server symbols, command run function, and JS modules as expected.
- Browser verification found a native timer receiver error (`Illegal invocation`)
  that Node's timers did not reproduce. Added a failing regression test enforcing
  browser timer binding, then fixed the default timer wrappers; regression passes.
- Commands: `gofmt -w cmd internal web`, `go test ./... -count=1`,
  `go test -race ./...`, `go vet ./...`, `go build ./...`, `make lint`,
  `make test-web` — passed. JavaScript: 11 tests passed. Also ran `node --check`
  on app/feed/model modules and `git diff --check`, all exit 0.
- Go cache access occasionally required approved execution outside the sandbox;
  the complete checks were repeated successfully. No dependencies downloaded.
- Ran `go run ./cmd/dashboard --source=sim --listen=127.0.0.1:8080`, then built
  `/private/tmp/lanos-dashboard-m2` for signal/restart checks. Application starts
  and serves live simulator updates without hardware or network services.
- Orca browser: observed SIMULATOR updates; measured browser frames at 1280×800,
  800×1280 and 390×844 with no horizontal overflow. At 1280×800 all content fits
  vertically. The host overrides its own viewport size, so exact sizes were
  verified in same-origin srcdoc frames rather than trusting the viewport API.
- Observed a simulated disconnect with `health=disconnected`, speed `—`, and
  source still SIMULATOR, followed by automatic return to fresh data.
- Sent SIGINT to the test binary: clean exit 0 with an active stream. Browser
  hid readings and displayed retry attempt 1/6. Restarted the server: browser
  recovered to `connected`, speed 0, SIMULATOR without a page reload.
- README documents commands, flags, phases, API, reconnect policy and hardware
  boundaries. No implementation blocker. `.codex/START_HERE.md` remains absent.
- Physical Teclast/Android performance, macOS SPP, adapter transport, ECU identity
  and PID support remain **untested**. M3/M4 protocol/serial tests are not claimed.
- Delivery branch: `feat/lanos-dashboard-m0-m2`, created at the user's request.
- Next: M3 in a separate task.

### 2026-10-24 — M3, GitHub Issue #1

- Read project instructions/docs and Issue #1 (no comments). Created branch
  feat/m3-vgate-probe. User's untracked test.py is untouched/excluded.
- Saved exact 13 reported exchanges with CRs/prompts and provenance in
  internal/elm327/testdata/vgate-2026-10-24.json. Baud and individual observation
  timestamps were not supplied and are not invented.
- Real fixture decodes A5, BE3EB811, RPM=0, coolant=78°C, speed=0 and no stored
  DTCs in that Mode 03 response. User reported Check Engine off; pending/permanent
  codes, current MIL and exact ECU identity are not inferred.
- Implemented stdlib-only macOS nonblocking raw 8N1 serial, context-aware I/O,
  explicit port/baud and name-only enumeration. Other hosts have serial stubs.
- Added read-only allowlist, serialized CR commands, 16 KiB response bound,
  per-command deadlines (initial 0100: 30s), prompt/echo/chatter handling and
  protocol/PID/DTC decoders. Errors distinguish adapter, ECU and transport.
- Timeout blocks writes until delayed prompt recovery, then full re-init/scan.
  Reopen also requires a recovery prompt. Retries/backoff are finite; no blind
  input flush, automatic Bluetooth pairing, VIN requests or ECU writes/clears.
- Discovery follows bitmap continuation bits and separates support from reading
  availability. NO DATA never becomes zero/unsupported. Each attempt rebuilds
  state; failed readings become stale and numeric values are cleared. ATRV is
  adapter voltage, distinct from ECU voltage. Unknown decoders are raw_only.
- CLI adds timestamped per-command raw/error audit, partial export on failure/
  cancellation, default recognizable VIN redaction and exclusive-create 0600
  JSON output. diagnostics/ is Git-ignored.
- Tests were added first, observed missing-symbol failures, then passed. Cases:
  real fixture, malformed/partial/conflicting frames, negative responses, DTC
  padding, PID formulas, fragmentation, concurrent serialization, late prompt,
  timeout/overflow, capability gating, unknown/unsupported/NO DATA, adapter vs
  ECU errors, re-init/reopen/retry exhaustion, cancellation/output/redaction.
  Serial I/O tests use anonymous pipes and pure settings, no physical devices.
- Passed: gofmt on changed packages; `go test ./... -count=1`; `go test -race ./...`;
  `go vet ./...`; `go build ./...`; `make lint`; `make test-web` (11 JS tests);
  `GOOS=linux GOARCH=amd64 go build ./...`. Initial sandboxed checks hit restricted
  Go cache/stdlib access; repeats with approved execution permissions passed.
- go.mod/Go minimum/dependencies unchanged. goimports unavailable; imports use
  gofmt-sorted stdlib/local groups. Built /private/tmp/lanos-obdprobe-m3.
  --help passed; --list-ports --json listed names including Android-Vlink without
  opening devices. No-argument launch correctly exited 1 with SPP/port guidance.
- Updated README/PROJECT; added capabilities.md and NEXT_CONNECTION.md with
  manual baseline/scan commands, evidence limits and reconnect guidance.
- No adapter/ECU commands or physical tests performed by the agent. No software
  blocker; physical Go probe operation, ignition-cycle recovery and additional
  ECU responses remain pending. Next: user car session, then separate M4.

### 2026-10-24 — owner comment: adapter model correction

- Read the new owner comment:
  https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184.
  The actual model is Vgate iCar Pro BLE 4.0 Dual, replacing the earlier incorrect
  Bluetooth 3.0 model description. Classic SPP via Android-Vlink on MacBook M3
  has already worked; the new Go implementation still needs its physical test.
- Corrected PROJECT, PLAN, README, capabilities, next-connection instructions
  and fixture metadata. Original command/response bytes are unchanged.
- Diagnostic reports now separate device.model/model_source/evidence_url,
  transport=serial_spp and the actual ATI adapter identity. The CLI defaults to
  the owner-reported model; --adapter-model overrides it, including empty for
  unknown. Library callers with no model information retain unknown context.
- Kept the existing context-aware transport interface. No BLE code, UUID/UART
  assumptions or concurrent Classic/BLE guarantees. M3 remains usable over SPP
  without waiting for BLE discovery.
- Added failing model/provenance/default tests before implementation, then
  verified owner/custom/unknown model handling and unchanged ATI identity.
- Passed: gofmt -w internal/probe cmd/obdprobe; go test ./... -count=1;
  go test -race ./...; go vet ./...; go build ./...; make lint; git diff --check.
- No Go version/dependency changes, hardware access or new blocker. User's
  untracked test.py remains untouched. Continue with the manual SPP session;
  BLE service/characteristic discovery is a separate future task.

### 2026-10-24 — physical attempt, ignition on / engine off

- User authorized read-only testing on the parked car, confirmed ignition on
  with engine off, and manually reconnected Android-Vlink. Later reported
  Bluetooth connects and disconnects after about 15 seconds. Other OBD
  applications are closed according to the user.
- Built /private/tmp/lanos-obdprobe-live. First sandboxed 38400-baud attempt
  could not open the device (operation not permitted); no commands were sent.
  Repeated with approved serial access: port opened, ATZ timed out with no raw
  bytes; bounded recovery/reopen attempts also received no bytes. A separate
  9600-baud attempt with no retries likewise received no ATZ response.
- Separate adapter-only helper: 38400 baud, 2s post-open pause, ATZ with 3s
  timeout also returned no bytes. This does not establish a settling-delay fix.
- Passive helper held the port open for 25s and sent zero bytes. Filtered
  system_profiler SPBluetoothDataType -json observations showed Android-Vlink
  not connected at +0.09s, connected at +10.16s, then not connected at +20.18s,
  before the helper closed the port. Thus that observed disconnect was not
  caused by closing the test process or sending ATZ. Exact disconnect time and
  underlying cause remain unknown.
- One controlled follow-up waited until macOS listed device_connected (+7.8s)
  before attempting ATZ with an 8s deadline. Again no raw bytes were received.
  Bluetooth-connected state alone does not establish a working SPP data path.
- lsof found no owner of the port after probes finished. This does not rule
  out all competing clients at other times. Python serial module is unavailable;
  no dependencies installed and the user's test.py was not executed or changed.
- Private, exclusive-create diagnostic reports are preserved in ignored
  diagnostics/ignition-on-20261024-*.json; no raw reports added to Git. Temporary
  diagnostic Go source removed. All test processes finished and ports closed.
- No ECU requests reached execution: no new PID, voltage, protocol or DTC
  observations. Empty untested DTC fields do not mean no stored DTCs. ECU
  identity/compatibility cannot be inferred from these transport failures.
- Blocker: adapter/SPP communication; no production fix applied without evidence.
  Next: user power-cycles Vgate and checks power stability; one bounded
  adapter-only check before any baseline/scan. Do not start the engine merely
  to diagnose Bluetooth, and do not run repeated long recovery loops.
- After removing the temporary helper, `go test ./... -count=1`, `make lint`
  (including go vet and formatting check) and `git diff --check` passed.
  Persistent changes in this follow-up are documentation only.
- Follow-up: user unplugged/reinserted Vgate and confirmed stable power light.
  Repeated the same connection-gated ATZ experiment at 38400 with 8s timeout;
  macOS listed connected at +0.14s, but the command again received zero bytes.
  Saved diagnostics/ignition-on-20261024-link-connected-002-after-power-cycle.json.
  Process finished and port closed. Power-cycling did not restore AT responses.
- Both available python3 and python3.13 lack pyserial. Asked how the previously
  successful test.py was launched; next compare that environment's serial setup
  (including flow/modem control) using one AT-only command, with no blind input
  flush or continued ECU polling after timeout. Root cause remains unresolved.

### 2026-10-24 — new owner BLE evidence and transport priority

- Read both new comments in Issue #1:
  https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6384173048
  and https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6384193400.
- Owner reports three BLE connect/query/disconnect sessions with ignition on,
  engine off, and three with engine running on a stationary car, without
  re-pairing. Observed IOS-Vlink service/RX notify/TX write UUIDs and response
  payloads are recorded in capabilities.md; they are not guessed UART UUIDs.
- All sessions returned 0100=BE3EB811 and ATDPN=A5. Engine-off readings were
  40°C / 0 RPM. Engine-running readings: 39°C / 959 RPM, 39°C / 935 RPM,
  38°C / 949.5 RPM; arithmetic mean approximately 947.8 RPM. First ATE0 echo+OK
  versus later OK is expected echo state, not a transport failure.
- Updated PROJECT/PLAN/README/capabilities and next-connection guidance:
  prioritize BLE for MacBook/Android, preserve SPP, reuse common ELM protocol
  and capability logic. No new hardware queries, BLE implementation, module
  changes or dependencies. Reports contain payload summaries, not exact CR/
  prompt/notification bytes; do not label them exact framing fixtures.
- This is owner-supplied physical evidence, not a successful Go BLE test.
  Android, sustained operation, engine restart, polling latency/rate and
  reconnect/stale behavior still need testing. M4 is not marked complete.
- Verification: `make lint` (go vet and formatting) and `git diff --check`
  passed; independently recalculated reported RPM/coolant and mean RPM using
  Python stdlib. Documentation only; no new live tests or Go behavior changes.
