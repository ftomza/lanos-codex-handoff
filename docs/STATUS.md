# Session handoff / status

## Current state
M3.3.5 is implemented and offline-verified on `feat/mil-analysis`: bounded
read-only MIL archive reader, reusable evidence/heuristic analysis, and
`milanalyze` Russian text/versioned JSON reports. Current saved P0420 replay
preserves partial context, final UNKNOWN and independent late freeze replies.
No transport/polling/fuel changes or new vehicle connection. M3.3.6 remains next.

2026-11-07 owner-recorded sessions: confirmed MIL OFF -> ON at 16:54:18 Moscow,
stored and pending P0420, freeze-frame cause P0420. Mode0A returns unsupported
7F0A11. Today's physical lamp state is not owner-confirmed; no subsequent OFF
is recorded. Context is retained with gaps, not a complete continuous window.
See diagnostics/2026-11-07-trip-review.md for evidence and fuel analysis.

2026-10-29 Check Engine follow-up: owner reports steady light and smooth idle;
physical Go BLE confirms MIL on but empty stored Mode 03 and pending Mode 07
lists. Cause is unresolved. Finite --read-pending support is implemented on
feat/pending-dtc (based on feat/fuel-analytics); no clearing occurred.
M6.1 is implemented and offline-verified on `feat/fuel-analytics` in this same
checkout: coherent fuel diagnostics/quality and IAT heuristics, bounded opt-in
JSONL history, shared streaming analysis/CLI and a saved-history dashboard page.
State v1, dependencies/Go version, ECU command set/schedule and standby behavior
are preserved. No physical M6.1/Android/consumption-accuracy claim is made.
M0–M3 complete. M4 live dashboard and M6 fuel computer are implemented,
offline-verified and merged into main via PR #3 (87f324e). The owner reports
a successful first combined LIVE run; no duration, readings or logs supplied.
The ignition-off waiting fix from fix/ecu-standby is integrated into main:
continuous LIVE waits and resumes after ECU/adapter availability returns.
Measured polling/latency, physical ignition recovery/endurance, fuel accuracy,
IAT validation and Android/device acceptance remain pending.
Default dashboard source remains sim; explicit
--source=elm327 selects native BLE or serial without simulated fallback.
The Go read-only obdprobe provides macOS native BLE and serial, shared ELM framing/recovery,
capability discovery, PID/DTC decoding, bounded reconnect and private JSON export.
Native Go BLE passed a physical baseline on 2026-10-24 with this MacBook/Vgate/
Lanos: RPM 889.25, coolant 52°C, speed 0, ATRV 11.0V, MIL off and no stored DTCs
in the Mode 03 response. All 12 commands succeeded; connection closed normally.
The subsequent native Go supported-PID scan also passed: 27 commands, one
attempt, 18 data PIDs read (14 decoded, four raw_only), no warnings. Bitmaps
BE3EB811/C0000000 were confirmed. MAP/IAT/load/throttle/trims/O2 are now observed;
the latest readings and exact payloads are documented in capabilities.md.
The --record trip grabber is implemented with private incremental JSONL,
scheduled capability-gated polling and bounded recovery. Owner logs on
2026-10-28 verify a 60s parked recording and a 14m36s drive on AI-92, with one
segment and no gap/recovery events. The longer recording ends failed after
engine-off/Ctrl+C, with the cleanup cause absent from its summary. IAT is
unverified (-40°C then constant +20°C); no validated real consumption was
calculated from these historical recordings.
Earlier Go SPP attempts received no ATZ response; their root cause is unresolved.

Owner correction: the device is **Vgate iCar Pro BLE 4.0 Dual**. Classic SPP
through /dev/cu.Android-Vlink on MacBook M3 is confirmed in the owner's comment.
New owner comments confirm BLE GATT discovery and real ECU responses on MacBook
M3: three engine-off and three engine-running sessions without re-pairing.
BLE is the priority direction and is implemented in Go using the approved
tinygo.org/x/bluetooth v0.16.0. Android and sustained reliability remain unverified.
The agent also repeated three successful engine-running BLE sessions using the
existing Python environment and the user's unchanged test2.py, followed by
60.17s of continuous RPM/coolant polling without observed errors or disconnects.
This verifies that diagnostic Python path, not the Go implementation or long-run
stability.

## Next action
Next MIL milestone is M3.3.6: use the offline report on a diagnostics screen.
M3.3.5 commands/limits are in MIL_DIAGNOSTICS.md; no more recording is required
to use it. Separate physical IAT/P0420 investigation and full M3.3.7 acceptance
remain independently authorized work. Historical next-action entries follow.

Owner approved merging fix/ecu-standby into main. The merged tree passed fresh
offline checks. Follow NEXT_CONNECTION.md for a separately agreed parked ignition
cycle test; no physical vehicle was accessed for this fix.
M6.1 requirements (1471a63) and ECU standby (7cc6aa8) remain in main.
The owner approved the design and implementation on feat/fuel-analytics.
Review that feature branch, then follow the new M6.1 section in
NEXT_CONNECTION.md for a separately agreed parked test and unattended recording.
Keep the existing live-experimental state path; add --fuel-record-dir to save
history. Old state totals cannot reconstruct a historical time series.
No vehicle connection is authorized or required for the offline implementation.

Owner approved M6; it is now merged into main with the new fuel
panel, persistent totals/coverage, local reset and reversible calibration.
Build with make build-dashboard; simulator starts with --source=sim. See
FUEL_COMPUTER.md for exact model assumptions, defaults and storage behavior.
For an agreed parked experimental LIVE run use --fuel-allow-unverified-iat and
--fuel-state=data/fuel/live-experimental.json. This does not validate IAT or fuel
accuracy; default running-engine LIVE speed-density stays unavailable without
that opt-in. No actual vehicle access was performed during implementation.

The owner completed a first combined LIVE run after merging PR #3. For a
separately confirmed follow-up parked vehicle session, the M4 command is:
`make build-dashboard`, then
`./bin/dashboard --source=elm327 --transport=ble --ble-name=IOS-Vlink --retries=0`.
Open http://127.0.0.1:8080. Do not run a recorder/other OBD client concurrently.
The owner approved the design and implementation; no new hardware connection
was made in this implementation session. Follow NEXT_CONNECTION.md for current
engine/ignition confirmation, 60s baseline, two browsers, timing/freshness,
controlled disconnect/recovery and Ctrl+C evidence. Change ignition only in an
explicitly agreed scenario. Detailed physical M4 recovery/timing acceptance,
Android and endurance remain pending.

The four instruments use independent freshness (RPM/speed 3s, coolant 35s,
ATRV 130s), plus global 3s ECU silence. LIVE now uses bounded reconnect bursts
and 10s idle pauses until canceled; retries=0 still waits for ECU/adapter return.
ECU standby uses sparse ATI/0100 checks and distinguishes missing adapter from
possibly-off ignition. No new command may precede pending-prompt recovery.
Configuration/permission/unknown initialization and real cleanup errors remain
terminal with HTTP running as LIVE/disconnected. Browser retries only restore
SSE/server access. Finite probe/record retry budgets are unchanged.
New recorder summaries retain
actual cleanup errors; offline helper cancellation/forced-close tests passed.
The exact old engine-off/Ctrl+C failure is still unknown and was not suppressed.

The two owner recordings remain analyzed in capabilities.md and the private
trip-analysis-20261028-001.json. Validate IAT (-40°C then constant +20°C) before
fuel estimation; do not replace it with an assumed temperature. OBD distance
9.49054 km versus rounded owner odometer 167→177 km does not justify calibration.
M6 now has tested estimators/accounting but still needs validated live inputs
and independent measured full-to-full refueling liters. ATRV is adapter supply,
not a direct generator measurement; verify its accuracy independently at battery
terminals. Old recordings are not presented as measured fuel consumption.

## Completed milestones
- M0 — module, package boundaries, commands, README and Makefile.
- M1 — nullable telemetry, freshness, cancellable source contract and simulator.
- M2 — local HTTP/SSE transport, embedded responsive UI and bounded reconnect.
- M3 — read-only diagnostic CLI, real-session fixture, discovery and recovery.
- M3 follow-up — native Go BLE transport, private export, baseline physically passed.
- M3 discovery follow-up — native Go supported scan physically passed; 18 data PIDs.
- M3 trip-recorder follow-up — offline-tested, then owner 60s/14m36s recordings analyzed; cleanup/IAT issues remain.
- M4 — merged/offline checks complete; first LIVE run reported successful by owner; ignition-off fix integrated into main, physical acceptance pending.
- M6 — merged/simulator acceptance complete; first combined LIVE run reported successful; live accuracy/IAT/calibration and visual/device acceptance pending.

- M6.1 — diagnostic recording, quality, IAT checks, offline CLI/HTTP and history UI implemented on feat/fuel-analytics; physical recording/Android/accuracy pending.

## Decisions and risks
- Local-first, read-only diagnostics.
- MacBook M3 is development host; Android Teclast P85 is target.
- Issue #1 observed /dev/cu.Android-Vlink, ELM327 v2.3 identity and A5 (KWP2000
  fast init). Ignition changes can break Bluetooth; future connectivity/recovery
  is not guaranteed. Android SPP is untested; Go serial communication has been
  attempted physically but has not yet received any adapter response.
- ECU identity remains unconfirmed. BE3EB811 and RPM/coolant/speed/Mode 03
  responses are known. 0120=C0000000 advertises only PID 21/22 in that range;
  PID 2F is absent and PID 40 continuation is clear. Higher ranges were not queried.
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

### 2026-10-24 — agent BLE test, engine running

- User confirmed the engine was running after agreeing to a stationary test.
  Used existing .venv/bin/python with bleak 3.0.2; installed no packages and
  changed neither user test.py nor test2.py. No SPP client or simulator started.
- Ran unchanged test2.py with unbuffered output through a 150s outer deadline
  and exclusive-create 0600 timestamped local log. Three BLE connect/query/
  disconnect sessions completed normally, without re-pairing, errors or timeouts.
  Commands: ATE0, ATL0, ATSP0, 0100, ATDPN, 0105, 010C. No ECU writes/clears.
- All three returned 0100=BE3EB811 and ATDPN=A5. Session 1: 0105=41055A
  (50°C), 010C=410C0DF2 (892.5 RPM). Session 2: 41055B (51°C), 410C0DE0
  (888 RPM). Session 3: 41055B (51°C), 410C0D73 (860.75 RPM).
- Exact response strings including CR/prompt, log receipt timestamps and
  session lifecycle are preserved privately in
  diagnostics/engine-on-ble-20261024T123131Z-sessions.log. These are physical
  observations made by the agent, unlike the earlier owner comment summaries.
- Followed with one continuous BLE connection using a temporary diagnostic
  variant in ignored diagnostics/, derived from unchanged test2.py. Validated
  the expected BE3EB811 bitmap before polling only 010C and 0105. Used a 60s
  loop, 0.2s pause per RPM iteration, coolant every fifth RPM, 3s measurement
  deadlines and a 120s process bound. No retries after command timeout in this
  single-session experiment. No production transport/parser was implemented.
- Continuous window 12:33:17.839–12:34:18.010 UTC (60.171s): 117 RPM responses
  and 23 coolant responses; no timeout/error/disconnect observed. RPM range
  829.5–908, mean 866.635; coolant 57→62°C. Observed rates with this schedule:
  RPM 1.944 Hz, all measurements 2.327 responses/s. These are not maximum ECU
  throughput claims. Initial 0100/SEARCHING took 5.129s outside that window.
- Measurement command elapsed times (monotonic, GATT write through prompt):
  median 249.4ms, p95 324.2ms (nearest rank), range 246.7–374.3ms. Raw response
  log timestamps mark wrapper receipt, not ECU capture time. Private artifacts:
  diagnostics/engine-on-ble-20261024T123309Z-continuous.py, corresponding .log,
  and diagnostics/engine-on-ble-20261024T123309Z-summary.json (all 0600).
- Offline analysis checked raw payload lengths/PID headers, prompt completion,
  decoded values, counts, clean session closure and absence of reported errors.
  Both processes exited 0 and closed their BLE connections. Told the user the
  test was finished and engine could be stopped; no further hardware queries.
- Limits: no deliberate disconnect/stale injection, engine restart, extended
  endurance, Android, new PID range or DTC tests. SPP root cause remains unknown.
  Next remains BLE transport + M4 with shared ELM logic, capability gating and
  freshness/reconnect tests. Do not label the Python experiment a Go live Source.
- Verification after documentation updates: `go test ./... -count=1`,
  `make lint` (go vet and formatting) and `git diff --check` passed. Repository
  changes are documentation only; raw reports/scripts remain private and ignored.

### 2026-10-24 — read-only voltage check, engine running

- User requested current generator voltage and confirmed engine still running.
  One Python BLE session, existing environment, derived diagnostic script only;
  original test.py/test2.py and production Go code unchanged. No packages added.
- ATE0/ATL0/ATSP0 returned OK. Three ATRV responses at 12:37:47–48 UTC were
  11.0V, 10.9V, 11.0V. This is adapter-measured supply at the OBD connector,
  not a measurement at generator terminals or an ECU battery-voltage PID.
- 0100 returned BE3EB811. 010C=410C0D43 decoded to 848.75 RPM, confirming
  engine activity during this session. 0120=4120C0000000 advertises PID 21/22
  only; 2F fuel level is not supported in that bitmap, and PID 40 continuation
  is clear. Did not send 0140/0142 or guess unavailable ECU voltage.
- ATRV is unexpectedly low for a charging conventional 12V system; it requires
  an independent multimeter check at battery terminals with engine running.
  Adapter calibration/wiring drop or charging-system issues cannot be separated
  by these responses; do not diagnose generator failure from ATRV alone.
- Session closed cleanly, child exit 0, no reported timeout/errors. Private 0600
  script/log: diagnostics/engine-on-ble-20261024T123744Z-voltage.py and .log.
  Next: verify voltage independently, then continue BLE/M4 work separately.
- Documentation checks: `make lint` (go vet and formatting) and
  `git diff --check` passed. No production behavior changed.

### 2026-10-24 — scope clarification: utility before dashboard

- User requested the application change, then clarified that the current focus
  is our utility. Narrowed work to BLE support in Go obdprobe, preserving the
  existing common ELM session/probe/report logic. Dashboard/M4 stays separate.
- Inspected CLI selection, serial factory, initialization, recovery, metadata,
  export and UI boundaries. No production code or dependency changes.
- Prepared concrete design with CLI flags, observed GATT profile, process
  lifecycle/deadline rules, transport bounds, report provenance and test coverage:
  docs/superpowers/specs/2026-10-24-obdprobe-ble-design.md. Recommended an
  embedded transport-only bridge to the existing bleak environment, with no Go
  module changes. Native Go BLE needs separate dependency approval/backend work.
- Await design/runtime choice before implementation, as required by the
  brainstorming skill. `git diff --check` passed; code/tests are unchanged.

### 2026-10-24 — native Go BLE library selection and approved version upgrade

- User rejected the Python bridge, requested native Go BLE, then explicitly
  authorized upgrading the project to Go 1.27. Updated go.mod to go 1.27.0;
  installed compiler is go1.27.2 darwin/arm64, CGO enabled, Xcode/clang available.
- Compared upstream docs/releases and versioned module metadata. Recommended
  tinygo.org/x/bluetooth v0.16.0 (Go >=1.25), with macOS CoreBluetooth support
  and repeated Adapter.Enable fix. v0.15.0 was compatible with the old minimum
  but lacks that fix. go-ble/ble explicitly does not actively maintain macOS.
- Updated the written design to native Go. Library initializes CoreBluetooth
  managers at package init and some blocking APIs lack contexts, so isolate it
  in our native Go helper started only for explicit BLE. Main CLI/offline tests
  retain no implicit Bluetooth access. This is a Go build, without Python.
- No new dependency added/downloaded yet. Await approval of the pinned library
  and its transitives before modifying dependency requirements. No hardware
  queries or native compatibility claims. Dashboard/M4 remains separate.
- Upgrade verification passed: go test ./... -count=1; go test -race ./...;
  make lint (including go vet and gofmt check); go build ./...; make test-web
  (11 tests); git diff --check. Build emitted a nonfatal sandbox stat-cache write
  warning; all commands exited 0. No source/test changes or new modules.

### 2026-10-24 — native Go BLE obdprobe implemented and physically tested

- User explicitly approved tinygo.org/x/bluetooth v0.16.0 with transitives and
  implementation of the reviewed native design; Go 1.27 upgrade was already
  authorized. Added pinned go.mod/go.sum requirements. TLS/cache sandbox failures
  were retried with approved execution; no Python dependency in production.
- Added bounded stdlib process transport, native Go helper behind the nativeble
  build tag, observed GATT UUIDs and shared ELM transport integration. Main CLI,
  --help, enumeration, serial mode and ordinary tests do not initialize
  CoreBluetooth. Mac helper is built with CGO; unsupported builds fail clearly.
- New flags: --transport=serial|ble, --ble-name, --ble-connect-timeout,
  --ble-helper. Reject mixed options; no silent fallback. BLE report source is
  elm327_ble/ble_gatt with profile metadata and no serial baud/port. Human output
  now includes adapter ATRV and status. BLE uses ATE0/ATL0/ATI/ATSP0 without ATZ;
  serial initialization, allowlist, discovery, decoding and recovery are reused.
- Tests were written first and observed failing on missing Config/transport/
  helper symbols, unsupported flags, absent physical fixture and missing human
  voltage output. Then implemented and passed. Coverage includes real raw
  responses, interleaved notification/ack, fragmented reads, ack identity after
  timeout, startup failure/cancel, EOF, bounds, close unblocking and forced child
  termination/reaping. Fixed a test assumption that fast child errors always
  occur after Open returns; both safe outcomes are accepted.
- Full checks passed: go test ./... -count=1; go test -race ./...; make lint
  (vet/format); make build-obdprobe; make test-web (11 tests);
  GOOS=linux GOARCH=amd64 go build ./.... Native helper compiled darwin/arm64.
  --help and --list-ports --json ran without opening devices. goimports remains
  unavailable; used gofmt and manually grouped stdlib/local imports.
- User reconfirmed parked car, engine running and permission for one short Go
  probe. Ran bin/obdprobe --transport=ble --ble-name=IOS-Vlink
  --ble-connect-timeout=30s --timeout=3s --initial-timeout=30s --retries=0
  --output=diagnostics/native-ble-engine-on-20261024-001.json. Complete, exit 0,
  one attempt, all 12 commands OK, clean close. No second hardware run started.
- Physical Go results: ATI ELM327 v2.3; 0100 BE3EB811; ATDPN A5; ATRV 11.0V;
  010C 410C0DE5 = 889.25 RPM; 0105 41055C = 52°C; 010D 410D00 = 0 km/h;
  0101 41010007E1A1 = MIL false / stored count 0;
  03 43000000000000 = no stored codes in that response. Pending/permanent codes,
  exact ECU identity and voltage accuracy remain unconfirmed. Low ATRV still
  requires independent battery-terminal measurement.
- Added exact physical response fixture: 30 Python BLE exchanges plus 12 from
  this Go report, with provenance and host receipt/completion timestamps;
  notification boundaries and ECU capture times are not invented. Full private
  report remains ignored/0600; user test.py/test2.py unchanged and excluded.
- Updated build/runtime instructions, capability evidence, dependency license
  notices and implementation plan. Dashboard remains simulator-only, M4 not
  completed. Next: supported scan and native endurance/reconnect tests, then M4.
- Final checks repeated after adding the native responses and human-readable MIL:
  full Go tests/race, lint, native build, 11 web tests, Linux cross-build and
  go vet -tags=nativeble ./cmd/obdprobe-ble all passed. Native backend was not
  queried again after the single authorized physical baseline.

### 2026-10-24 — native Go supported-PID discovery, engine running

- User asked what is available and freshly confirmed parked car, engine running
  and permission to scan. Ran exactly one read-only native Go BLE scan:
  `./bin/obdprobe --transport=ble --ble-name=IOS-Vlink
  --ble-connect-timeout=30s --timeout=3s --initial-timeout=30s --retries=0
  --scan-supported --output=diagnostics/native-ble-supported-20261024-001.json`.
- Window 17:42:05.039–17:42:20.557 Europe/Moscow (15.52s); complete, exit 0,
  one attempt, all 27 commands ok, no warnings/errors/timeouts observed.
  Process exited and closed BLE. No further hardware queries or writes/clears.
- Confirmed 0100=BE3EB811 and 0120=C0000000, protocol A5, ATI ELM327 v2.3.
  Read all 18 advertised data PIDs: 14 available/decoded, four raw_only;
  PID 20 is the separate continuation bitmap. No unsupported PID requested.
- New readings: RPM 936.5, speed 0 km/h, coolant 31°C, MAP 41 kPa, IAT 17°C,
  throttle 0%, calculated load 5.098%, STFT bank1 0%, LTFT bank1 -10.15625%,
  timing advance 4°, O2 sensors 1/2 both 0.455V, OBD standard code 1.
  O2 sensor1 trim 0%; sensor2 byte FF has no available trim interpretation.
  MIL off/stored count 0; Mode 03 contains no stored DTC. No pending/permanent
  requests. Each reading is sequential, not a simultaneous snapshot.
- Positive raw-only payloads: 0103=41030100, 0113=411303, 0121=41210000,
  0122=41220000. Do not invent numeric values for absent semantic decoders.
  MAF 10 and fuel level 2F absent; bit 40 clear, so 0140/0142/015E not sent.
- ATRV again 11.0V. Independent battery-terminal multimeter check still needed;
  no generator diagnosis from adapter supply. Single O2/trim samples do not
  diagnose engine faults. MAP/IAT/RPM inputs do not validate M6 estimation.
- Report remains ignored and private (0600); offline validation checked command
  count/status/prompt completion, bitmap chain, support/status counts and mode.
  Updated capability matrix and next-connection guide. Production code, tests,
  dependencies and user test.py/test2.py unchanged; M4 remains separate.
- Verification after documentation updates: `go test ./... -count=1`,
  `make lint` (go vet/formatting) and `git diff --check` passed. All Go tests
  are offline; no additional Bluetooth session was started by verification.
- Next: independent voltage measurement, optional offline semantic decoders,
  then planned native Go endurance/reconnect/stale tests before M4 integration.

### 2026-10-24 — proposed trip recorder for fuel-computer research

- User requested an unattended grabber to start before a future drive. Reviewed
  existing CLI, probe lifecycle, capability/decoder boundaries and M6 requirements.
- Prepared a proposed design extending native Go obdprobe with explicit --record,
  capability-gated scheduled polling, private incremental JSONL, command timing,
  raw responses, bounded recovery, recording gaps and graceful termination.
  Defaults proposed: current MacBook/BLE, 4h bound, 256 MiB file-size limit.
- Core inputs: RPM/MAP/speed; supporting IAT, coolant, throttle, fuel-system
  status, trims, O2, load, timing, MIL and adapter voltage. No fuel estimate or
  dashboard integration in this utility follow-up. Logs alone cannot calibrate
  fuel volume: independent full-to-full refueling and odometer data are needed.
- Design self-review checked scheduling overload, no catch-up bursts, bounded
  memory/file/retry behavior, prompt recovery, existing CLI compatibility and
  separation of host timestamps from ECU capture times.
- Await owner review as required by the brainstorming skill. No production code,
  dependency changes or hardware access. No new tests run for this design-only
  change; git diff --check passed. User test.py/test2.py remain untouched.

### 2026-10-24 — trip recorder implemented after owner approval

- Owner approved the written design. Implemented on feat/fuel-trip-recorder
  using the inline plan docs/superpowers/plans/2026-10-24-trip-recorder.md.
  No Go version/dependency changes, Python or dashboard integration.
- Added explicit --record/--duration to obdprobe; default 4h, maximum 24h,
  generated exclusive 0600 diagnostics/trip-*.jsonl or explicit new --output.
  Existing finite probe/report behavior and native helper build remain usable.
- Shared initialization/discovery, parser/allowlist, PID/DTC/ATRV decoders and
  recovery. Core RPM/MAP/speed plus oldest-due supporting inputs; one command
  at a time, no catch-up burst, no unsupported PID queries. Actual command wall
  timestamps and monotonic offsets/latencies are recorded; zero timing fields
  are explicit in the streaming schema.
- JSONL schema v1 records configuration/revision, owner-reported displacement,
  capability evidence, samples/raw/errors, gaps/recovery, segment boundaries
  and final counters/polling/unobserved time. This is host timing, not ECU
  capture time or simultaneous snapshots. Polling time is not fuel-data coverage.
- Writer syncs every event before the next request (stronger than the minimum
  periodic-sync design), stops on disk/sync failure and caps output at 256 MiB
  with final-event reserve. No accumulated trip command/warning lists in RAM.
  Shared VIN redaction preserves integer precision; private logs stay ignored.
- Retry budget is total across the recording, default two additional attempts.
  Timeout/reopen still requires read-only prompt recovery before initialization;
  no blind flush, guessed ECU writes, clearing, VIN or pending/permanent queries.
  Duration and SIGINT/SIGTERM stop cleanly; close/output/transport errors surface.
- Tests written first failed on missing writer/recording/CLI APIs, then passed.
  Regression tests also caught duplicate segment_end after cancellation during
  backoff, unsynced events before long waits, missing zero timing fields and
  advertised-but-unavailable required inputs; each was corrected and verified.
  Repeated NO DATA clears previous measurements rather than preserving a value.
- Offline coverage includes real-fixture CLI/ELM/JSONL replay, malformed/NO DATA/
  raw-only readings, in-command cancellation, timeout/reopen prompt ordering,
  retry exhaustion, disk/short-write/sync/close failures, exclusive files,
  generated names, redaction, file bounds and a simulated one-hour recording
  with over 10,000 commands but no retained command/warning history.
- Initial race run exposed an overly short 1ms fake-adapter deadline on a normal
  response under instrumentation. Raised only test deadlines to 1s/100ms;
  go test -race ./internal/probe -count=3 and subsequent full race checks passed.
  Production timeout remains 3s.
- Final passed: go test ./... -count=1; go test -race ./...; make lint
  (go vet/gofmt); make build-obdprobe; GOOS=linux GOARCH=amd64 go build ./...;
  go vet -tags=nativeble ./cmd/obdprobe-ble; git diff --check. --help and
  --list-ports --json passed without opening devices. goimports unavailable;
  used gofmt and checked stdlib/local import groups manually.
- Added docs/TRIP_RECORDING.md and updated README/NEXT_CONNECTION. Explained
  parked startup/stop, unattended operation, MacBook awake/power conditions,
  actual sampling limits and full-to-full calibration notes. No hardware
  commands sent or new physical compatibility/endurance claim. test.py/test2.py
  untouched/excluded. M4 and M6 remain separate, incomplete milestones.
- Next: owner short parked recording, then ordinary unattended trip plus fuel
  type/odometer/refueling notes; inspect actual sample intervals and gaps before
  constructing/calibrating the speed-density fuel estimator.

### 2026-10-28 — offline analysis of two owner recordings

- Owner supplied two local native Go BLE JSONL recordings (build 79fec0b),
  clarified gasoline AI-92 and large-trip odometer 167→177 km, then confirmed
  engine stopped before Ctrl+C. No new adapter/ECU access by the agent.
- Short file: trip-20261028T052932Z-850540930.jsonl, 212 valid lines, 60.037s,
  207 commands, 206 ok and one canceled-by-duration timeout at the end;
  recording_end reason duration, one segment, zero gap/recovery events.
  Speed always zero, RPM 943.25–994.75, coolant 25→31°C, ATRV 13.7V.
- Large file: trip-20261028T053157Z-2657108889.jsonl, 3393 valid lines,
  876.350s / 14m36s, 3388 commands: 3383 ok, four NO DATA after RPM reached
  zero, one final context-canceled command. One segment, no gap/recovery events.
  recording_end exists but reason failed; exact cleanup error is not exported.
  Source review shows a cleanup error can override normal cancellation. Do not
  claim a proven cause or clean helper shutdown without its terminal error.
- Large-trip readings: speed 0–64 km/h; RPM 0–3101.75; MAP 27–102 kPa;
  coolant 42→86°C (minimum 41°C); ATRV initially 12.3V, later 13.2–13.6V.
  Historical 11V is not reproduced here; adapter voltage accuracy remains
  independently unverified. MIL false/count 0 in 14 readings; initial Mode 03
  contains no stored DTC, pending/permanent untested.
- Each core PID has 840 successful responses, 839 after discovery. Polling
  interval median 1.020s/p95 about 1.050s; speed maximum 1.154s. IAT median
  8.251s, maximum 11.220s. No observed polling interruption before shutdown;
  this does not validate forced reconnect, Android or longer endurance.
- OBD speed integral 9.4905368 km using same-segment trapezoids with dt ≤5s,
  no extrapolation; 860.358s covered. Excluded the initial 6.390s scan→polling
  interval. Versus owner 10 km: -0.50946 km/-5.095%; odometer resolution and
  precise interval boundaries unknown, so no correction factor applied.
- IAT raw 410F00: all seven short-file samples and first 34 of 101 large-file
  samples. At elapsed 298.695s it changes to 410F3C, then stays exactly 20°C
  for 67 samples. Decoder matches raw; sensor validity/fallback behavior is
  unknown. Do not diagnose a failed sensor or silently substitute temperature.
- Conservative alignment: prior MAP/speed age ≤3s and IAT ≤15s, provisionally
  excluding -40°C. Of 833 engine-on polling RPM points, 556 provisionally usable,
  276 have unresolved IAT, one lacks a fresh input. Remaining IAT accuracy and
  VE/mixture modeling are not verified. No fuel volume/L100km inferred from
  AI-92/odometer alone; independent refueling liters are still required.
- Python stdlib offline validation checked JSON/sequence/schema/monotonic
  timing, final events, raw values and independently decoded 3213 numeric
  readings in the large file. Private aggregate + owner notes saved exclusively
  as diagnostics/trip-analysis-20261028-001.json (0600). Original SHA256s:
  short a0954d207450292c902fbcb4081d8f00893f336f96b182965d1f26865d7ad346;
  large ee6fdf136b6a29bc82dd16b3779d7c187623a98db17ceceaeb39b7f1b83de699.
- Updated physical evidence/status/guidance. Production code/dependencies and
  user test.py/test2.py unchanged; raw/aggregate files remain ignored/private.
  No Go tests rerun for this documentation-only analysis; git diff --check and
  data integrity/numeric checks passed. M4/M6 remain incomplete.
- Next: diagnose IAT and cancellation cleanup offline/at a separately agreed
  parked session as needed; then decide estimator quality rules and collect
  independent full-to-full fuel measurements before claiming fuel accuracy.

### 2026-10-28 — live dashboard scope and proposed design

- Owner requested real data in the dashboard and a new branch, calling it M6.
  Clarified against PLAN: this is live integration M4, while fuel calculation is
  M6. Created feat/live-dashboard from be86564; no renumbering or fuel estimator.
- Reviewed current Source/Snapshot, server/SSE lifecycle, simulator-only CLI,
  UI source text/watchdog, and recorder polling/capability/recovery boundaries.
  The current global timestamps cannot safely express slow coolant/voltage
  freshness independently of fast RPM; per-signal metadata is required.
- Prepared a concrete proposed design: native internal/live Source reusing one
  shared probe stream, explicit elm327/BLE flags and build target, four existing
  instruments with LIVE labels, per-field expiry, bounded queues and retries,
  and HTTP remaining available after source retry exhaustion. Preserve sim,
  recorder/finite reports and the existing read-only/provenance constraints.
- Compared direct Source reuse, tailing a recorder file and a duplicate BLE
  stack; recommend direct reuse. IAT is not used to invent fuel values. Known
  cleanup failure remains explicit; improve diagnostics and fix only a defect
  reproduced by offline lifecycle/helper tests during integration.
- Self-reviewed CLI defaults, no implicit hardware opening, listener-first
  startup, independent field timestamps/TTLs, browser silence/retry behavior,
  cancellation and offline acceptance. Exact design is in
  docs/superpowers/specs/2026-10-28-live-dashboard-design.md.
- Await owner design review as required by brainstorming before implementation.
  No production code/dependency changes, hardware queries or new tests; this is
  a design-only change. git diff --check passed. User test.py/test2.py and all
  private recordings are untouched/excluded. M4/M6 not marked complete.

### 2026-10-28 — M4 live dashboard implementation, offline acceptance

- Owner approved the design; implemented inline on feat/live-dashboard using
  docs/superpowers/plans/2026-10-28-live-dashboard.md. M4 is live integration;
  M6 remains the separate fuel computer. No dependency/Go-version changes.
- Shared probe Runner.Stream reuses recorder allowlist, initialization,
  capability gating, schedule and finite total retry budget without a recording
  time limit/history. Record retains its original duration limit and behavior.
  RecordingSummary now exports actual terminal/cleanup error when available.
- Added internal/live Source: one worker, latest immutable snapshot, bounded
  storage, cancellation/join and real Close error propagation. Discovery/gap/
  recovery clears values; terminal failure publishes disconnected and keeps
  HTTP available. Closed-source final lifecycle callbacks are harmless; a
  failing regression reproduced the former spurious cancellation sink failure.
- Added signal provenance/support/status, own host response/receipt timestamps,
  TTL and flat nullable API compatibility. RPM/speed 3s, coolant 35s, ATRV 130s;
  global silence remains 3s. New PID replies cannot refresh other timestamps.
  Run-wide commands/errors/gaps/recovery-event counters retain no history.
- Explicit elm327/BLE/serial flags and make build-dashboard; sim remains default.
  Help/invalid config/bind failure cannot open hardware. HTTP binds first.
  One hardware stream serves all SSE clients; no browser-triggered BLE retries.
- Preserved UI layout; dynamic UNKNOWN/SIMULATOR/LIVE notice and notes, honest
  adapter-supply wording, per-field browser expiry and unavailable statuses.
  Only fresh numbers drive arcs/bars/warnings. Live failure never displays the
  simulator's six-second outage explanation. Timestamps identify host timing.
- Tests written first failed for new Stream/domain/source/CLI/browser APIs.
  New browser tests exposed old flat values overriding stale signal metadata;
  fixed using independently validated values. An asynchronous terminal-source
  test initially consumed a gap before final status; synchronized worker
  completion to check the actual terminal contract without a timing race.
- Offline integration replays saved Issue #1 response bytes through the real
  Stream/Source/HTTP/SSE stack, fragmenting notifications. Two clients share one
  open; unsupported/write/VIN commands absent, zero RPM stays measured zero,
  terminal errors keep HTTP alive and close the transport exactly once.
  Synthetic ATRV/continuation inputs are identified as test data only.
- Actual fake helper processes cover canceled reads/writes, clean close/reap,
  forced bounded termination and retained nonzero exit/stderr. No reproduced
  native-helper production defect; do not infer the old missing cleanup cause.
- Passed: go test ./... -count=1; go test -race ./...; make lint (go vet/gofmt);
  make test-web (17 tests); make build-dashboard; make build-obdprobe;
  GOOS=linux GOARCH=amd64 go build ./...;
  go vet -tags=nativeble ./cmd/obdprobe-ble; node --check for app/model/feed;
  dashboard/obdprobe --help; obdprobe --list-ports --json; git diff --check.
  Go cache restrictions required approved execution outside sandbox. goimports
  unavailable; gofmt applied and import grouping reviewed manually.
- Built simulator smoke: GET /, SSE with simulator/connected, SIGINT while SSE
  open, exit 0. No physical BLE/serial device opened. New browser visual/device
  validation was not performed; existing layout/CSS preserved.
- README, PROJECT/PLAN, approved spec/plan and NEXT_CONNECTION updated with exact
  commands, polling/freshness/retry limits and pending physical test checklist.
  .codex/START_HERE.md remains absent. User test.py/test2.py and private logs
  remain untouched/excluded. No ECU writes/clears, physical compatibility or
  calibrated fuel estimate claimed. Next: separate parked LIVE acceptance.

### 2026-10-28 — M6 fuel computer scope and proposed design

- Owner requested the fuel computer and its dashboard display in a fuel branch.
  Created feat/fuel-computer from c5c674e so the new work includes M4 LIVE source.
- Read AGENTS/PROJECT/PLAN/current STATUS, shared source/schedule/domain,
  simulator/server/UI and capability/recording evidence. MAF/direct fuel PID
  absent on observed Lanos; IAT remains unverified. No fresh hardware access.
- Proposed three strategies (direct ECU, MAF, speed-density), nominal gasoline
  assumptions, explicit live unverified-IAT opt-in, monotonic bounded accounting,
  matched-interval average, persistent source-specific totals, revisioned local
  reset and reversible full-to-full calibration, API/UI and offline acceptance.
- Compared integrated experimental/live support, simulator-only and waiting
  for direct ECU flow. Recommended integrated module with honest unavailable/
  experimental labels; do not substitute IAT, infer DFCO or calibrate from
  odometer endpoints without independent liters.
- Wrote and self-reviewed docs/superpowers/specs/2026-10-28-fuel-computer-design.md.
  Review covers units, coverage denominator, live/sim state separation, restarts,
  persistence errors, local mutations and unsupported command gating.
- Awaiting one owner spec approval as explicitly required by brainstorming.
  No implementation or dependency change. Documentation-only verification:
  git diff --check; Go tests not rerun. User test.py/test2.py and private logs
  remain untouched. M6 remains unimplemented; M4 physical acceptance pending.

### 2026-10-28 — M6 fuel computer implementation after owner approval

- Owner approved the written spec. Implemented inline in feat/fuel-computer,
  based on c5c674e, following docs/superpowers/plans/2026-10-28-fuel-computer.md.
  No dependency/Go-version change, new hardware access or ECU writes/clears.
- Added internal/fuel with direct ECU, MAF and speed-density strategies, explicit
  gasoline/VE assumptions and input bounds. Availability/provenance/confidence
  are explicit. Current Lanos has no advertised MAF/fuel-rate PID, so neither
  is requested. Live speed-density requires unverified-IAT opt-in; -40 C and
  other out-of-model values stay unavailable, no assumed temperature or DFCO.
- Extended live signal metadata with MAP/IAT/MAF/fuel rate and reconnect generation.
  Extended simulator with labelled synthetic inputs. Continuous Stream alone
  adds supported MAF/flow to core; --record keeps its original schedule/schema.
- One shared computer uses source updates plus a 250ms monotonic tick. Fresh
  endpoint trapezoids, <=3s dt, generation/time checks and independent expiry
  prevent extrapolation across gaps. Duplicate timestamps cannot double-count.
  Separate raw fuel, distance and jointly covered totals prevent dilution of
  average by kilometers with no fuel. Average available after 0.1 covered km.
- Source-specific private versioned state uses exclusive sidecar lock, strict
  schema/numeric/config checks, 0600 temp + fsync + atomic rename and <=5s
  checkpoint plus normal close/mutations. Startup errors precede source Start.
  Restart retains totals, drops anchors and marks unobserved coverage unknown.
  Corruption/unknown lock is not silently removed/reset. Runtime write failures
  remain visible and failed mutations preserve prior committed state.
- Local same-origin JSON reset/calibration endpoints validate bounded body and
  revision. Calibration requires exact full-to-full confirmation, >=10 covered
  km and complete interval, factor 0.25..4. Raw totals are unchanged; undo/default
  supported. A regression reproduced factor drift while polling continued
  between preview/apply; a matching preview token now fixes the displayed factor.
- Added dashboard fuel panel: L/h below threshold or without speed, L/100km above
  5km/h by default, covered average, liters/km since reset, model/IAT/coverage/
  persistence notes, reset confirmation and calibration preview/apply/undo.
  Instantaneous values expire independently, historical totals stay labelled.
  Added compact landscape CSS and narrow-screen grid; native browser/device
  visual validation has not been performed and remains pending.
- TDD regressions included stale inputs, numeric boundaries,
  gaps/restarts/generation/model changes, durable/reversible local controls,
  HTTP origin/JSON/revision failures, preserved recorder scheduling, UNKNOWN
  source labels and 404 on newly embedded JS modules. Corrected and verified.
- Passed: go test ./... -count=1; go test -race ./...; make lint; make test-web
  (22 tests); make build-dashboard; make build-obdprobe; Linux cross-build;
  go vet -tags=nativeble ./cmd/obdprobe-ble; CLI --help; changed JS syntax checks;
  git diff --check. Cache restrictions required approved execution outside sandbox;
  goimports remains unavailable, used gofmt and manual import grouping review.
- Built simulator smoke verified GET/static fuel modules, real computed flow and
  L/100km transition, positive liters/km, two SSE clients, revisioned reset,
  cross-origin/stale revision/short-calibration rejection, 0600 persistence,
  restart with unchanged reset time + incomplete marker, and SIGINT exit 0 with
  streams open. A separate DOM-stub HTML/app runtime smoke exercised numeric
  rendering and reset/calibration dialogs with token submission; not visual
  browser/device acceptance. All temporary state was isolated from owner data.
- Updated README/PROJECT/PLAN/FUEL_COMPUTER/NEXT_CONNECTION and approved spec/plan.
  .codex/START_HERE.md remains absent. Owner scripts/private recordings and fuel
  state are excluded. No real fuel-accuracy or complete-trip claim from old logs.
- Next: separately confirmed parked LIVE/dashboard baseline, verify IAT source,
  then controlled coverage/reconnect/shutdown tests and independent full-to-full
  calibration only over a valid matching interval. Android and visual checks
  remain pending. Ordinary vehicle operation must not depend on this dashboard.

### 2026-10-28 — First LIVE feedback and proposed ECU standby fix

- Owner reports the merged M4/M6 dashboard worked successfully on the car.
  This is owner-reported functional evidence, not measured polling/latency,
  fuel accuracy, IAT validity or endurance/reconnect acceptance. No new raw
  readings/duration supplied. Fuel gauge second tick is not a measured volume.
- Owner reports restart advice with adapter powered and ignition off, both
  at startup and after switching ignition off. Inspected live Source/events,
  continuous recorder, scan/PID error handling, session recovery and telemetry
  health. Stream has a terminal total retry budget; capabilities/AT responses
  can mark connected despite absent ECU, and framed NO DATA does not recover.
- Created fix/ecu-standby from main 87f324e. Proposed sparse ECU standby and
  separate adapter recovery, bounded retry bursts, unchanged finite Run/Record,
  preserved prompt synchronization and fuel gap accounting. Alternatives and
  acceptance are in the written spec. Await owner approval before code changes
  as explicitly required by brainstorming. Self-review checked scope, state
  transitions, retries=0 semantics, safe recovery and hardware claims.
- Commands: git status --short --branch; targeted source/document inspection;
  git switch -c fix/ecu-standby (sandbox blocked Git ref creation, approved
  escalation succeeded). .codex/START_HERE.md remains absent.
- Documentation only: git diff --check; no Go/Node tests rerun and no vehicle
  accessed. test.py/test2.py, private recordings and fuel state untouched.
- Next: owner review, implementation plan and failing regressions, then code,
  offline checks and separately agreed parked ignition-cycle acceptance.

### 2026-10-28 — Approved ECU standby implementation, offline acceptance

- Owner approved the written design; implemented inline on fix/ecu-standby
  following docs/superpowers/plans/2026-10-28-ecu-standby.md. No dependencies,
  Go version, fuel model, state schema, ECU allowlist or ADAS changes.
- Continuous Stream alone has an availability supervisor: missing first bitmap
  or three failed core rounds enters ECU standby; one absent PID and measured
  zero RPM remain valid cases. Sparse ATI/0100 checks avoid repeated full scans
  while ECU is absent. No-core profiles check 0100 on the idle interval.
- Transport recovery uses 1+Retries bursts, capped existing backoff and 10s
  pauses between bursts, including retries=0. Waiting remains cancelable.
  Timeout retains the session for read-only Recover; reopen retains needSync.
  No write/reinitialization before consuming a pending prompt. If the prompt
  never arrives, manual adapter intervention may still be necessary.
- LIVE Source publishes distinct ECU/adapter/synchronization messages with
  connecting health. Actual Mode 01 evidence alone updates global freshness;
  capabilities without ECU evidence and AT replies cannot create connected.
  Independent TTLs and new generation after recovery prevent old live values
  or fuel integration across gaps. Historical trip totals/reset/calibration
  survive; instantaneous fuel disappears and coverage becomes partial.
- BLE helper distinguishes expected disconnect/target-unavailable with optional
  typed wire codes after successful cleanup and exits cleanly for those cases.
  Native scan gets a cooperative deadline before the parent's hard bound.
  Unknown/config/permission errors, malformed framing, sink and actual cleanup
  failures remain terminal. Joined errors cannot hide a fatal cause behind a
  temporary error. Both binaries must be rebuilt together.
- Initial regressions reproduced terminal retries=0, false capabilities from
  NO DATA and lack of ignition-off transition; Source regressions reproduced
  AT activity refreshing ECU health. Tests were added before the fixes.
  A helper startup-loss test exposed internal context cancellation hiding the
  real disconnect; retained the callback cause. Joined-error regressions
  exposed and corrected masking of fatal permission/cleanup failures.
- HTTP/SSE integration passed startup without ECU -> 10s standby -> live data
  for two actively consuming browsers, one adapter open, new generation and
  clean cancellation. A first test left its second socket unread for 10s and
  correctly hit the server write deadline; fixed the test to consume both.
  Fuel integration verifies retained totals and no bridge even when a slow
  consumer skips standby. Finite Run/Record and existing cleanup tests pass.
- Passed: go test ./... -count=1; go test -race ./...; make lint;
  make test-web (23 tests); make build-dashboard; make build-obdprobe;
  go vet -tags=nativeble ./cmd/obdprobe-ble; git diff --check.
  Initial race run exposed a test reusing a deadline after waiting for a race
  helper process to exit; fresh context fixed it, full rerun passed. Build-cache
  sandbox warnings were followed by successful approved builds outside sandbox.
  goimports is unavailable; gofmt and manual import grouping review applied.
- Built simulator smoke passed HTTP page/snapshot, computed fuel, two SSE
  clients, SIGINT exit 0 and private 0600 state in an isolated temporary folder.
  Owner fuel state and private recordings untouched. Documentation/help now
  explain new LIVE retry semantics and the next parked ignition-cycle test.
- No physical BLE/serial connection, ECU request or ignition change performed.
  Vgate sleep/wake, physical automatic recovery, measured timing, endurance,
  Android and fuel accuracy remain unverified. test.py/test2.py are excluded.
  Next: commit/push feature branch, review, then separately agreed parked test.

### 2026-10-28 — M6.1 plan committed; ECU standby integrated into main

- Owner added M6.1 requirements and asked to commit them in main and continue
  in the existing repository folder. Committed PLAN.md only as 1471a63 and
  pushed main. Temporary stash transferred the exact 116-line addition safely;
  it was restored and removed. No additional worktree was created.
- Owner then approved integrating fix/ecu-standby. Merged 75dc822 into main
  without conflicts, preserving the complete M6.1 addition and ADAS plan.
- Fresh merged-tree checks passed: go test ./... -count=1;
  go test -race ./... -count=1; make lint; make test-web (23 tests);
  git diff --cached --check. Dashboard/native helper rebuilt with approved
  Go-cache access. No device was opened and physical ignition recovery remains
  pending. User scripts/private recordings/fuel state are untouched.
- M6.1 context contains owner-provided trip totals: 9.569 km, estimated 0.9369 L,
  9.79 L/100km, 98.4% observed-time coverage, speed_density, factor=1 and
  incomplete/process_restart/unavailable_input. These are reported estimates,
  not independently measured liters or a validation of IAT/VE/consumption.
- Next: advance feat/fuel-analytics to this main in the current folder;
  prepare the M6.1 recording/analysis/UI design before implementation.

### 2026-10-28 — M6.1 design on combined main

- Advanced feat/fuel-analytics to main 7cc6aa8 in the existing working folder;
  no additional worktree. main now contains both M6.1 requirements and the
  ECU standby fix, including fresh merged-tree verification above.
- Read M6.1/PROJECT/STATUS/FUEL_COMPUTER and inspected Source, existing PID
  decoders/schedule, estimator/accounting/API and UI. Existing Stream already
  reads throttle/trims/O2; exposing those responses needs no new ECU commands.
- Compared integrated coherent recording with an external SSE recorder;
  proposed integrated opt-in JSONL, bounded rotation/queue, separate history
  state/lock, shared CLI/HTTP analysis, additive input quality/intermediates,
  IAT heuristics and a saved-history dashboard page. Existing fuel state v1,
  formulas, opt-in behavior, dependency versions and ECU schedule remain intact.
- Written spec: docs/superpowers/specs/2026-10-28-fuel-analytics-design.md.
  Inline self-review checked retention/partial history, process time boundaries,
  calibration/reset jumps, source separation, timestamps and cold-start evidence.
  Limited recording intervals to <=2s to fit the existing 3s accounting bound.
- Design-only changes: git diff --check. No new M6.1 tests or implementation;
  the successful Go/race/lint/browser/build checks above apply to merged main.
  No vehicle queries or access to owner scripts/private state/recordings.
- Await owner approval of the concrete written design as required by
  brainstorming; then write implementation plan and execute regressions/code
  in this same feature branch and workspace.


### 2026-10-28 — M6.1 implementation after owner approval

- Owner approved the design and said «Да, погнали». Implemented inline in the
  same checkout on feat/fuel-analytics, based on main 7cc6aa8 and design 22573b5.
  No new worktree, no real adapter/ECU queries, no access to owner scripts or
  private trip/state files. The old .codex/START_HERE.md is absent in this tree;
  current AGENTS.md and PROJECT/PLAN/STATUS were consulted.
- Added optional coherent immutable Computer diagnostics, actual speed-density
  terms/units, nullable signal ages, input quality and bounded IAT heuristics.
  Preserved -40°C evidence, model limits and explicit LIVE IAT opt-in. Startup
  IAT can be investigated during connecting without enabling fuel calculation.
  Cold comparison requires an owner annotation and does not reopen on reconnect.
  Existing throttle/trims/O2 replies are exposed with independent TTLs; probe
  scheduling/allowlist/commands and the fuel state v1/fingerprint are unchanged.
- Added fuelhistory JSONL v1 with 256-row nonblocking queue, private 0600 parts,
  new 0700 directories, exclusive sidecar lock, atomic fsynced bounded manifest,
  64KiB rows and default eight 16MiB parts. Only owned retired parts are removed.
  Recorder errors stop recording visibly while source/HTTP/M6 remain running.
  Close returns failures; a still-writable footer preserves the failure cause.
  Missing referenced parts/corrupt metadata never reset the catalog silently.
- Shared cancellable streaming analysis serves fuelanalyze and configured-only
  HTTP catalog/report endpoints. Counters are authoritative only across valid
  comparable intervals; reset/factor/model/generation/sequence/time boundaries
  and >3s intervals cannot create fuel/distance. Includes coverage/gaps, known-
  speed weighted average, idle/moving/off/unknown, bins/mixed ranges, segments,
  IAT flags, markers and sampled-flow comparison. Bounded issue/segment/chart
  output; read-only analysis never advances fuel accounting or rewrites state.
- Separate analytics.html uses saved session/range data and six Canvas series.
  Gaps remain broken; quality, source, retention and downsampling are explicit.
  Small-budget regression caught a potential reduction loop at 1–2 points:
  one point now keeps newest, two keep extrema with conservative breaks.
  Filtered ranges keep lifecycle without false sequence gaps. Empty saved
  sessions do not appear as zero consumption. No JS/Go dependencies were added.
- Regression coverage includes formulas/units, freshness/missing inputs, distinct
  IAT timestamps, startup -40→20, constant/jump/cold checks, control boundaries,
  immutable observations, old state/API, rotation/source/locks, runtime write
  error/queue overflow, corrupt/partial history, cancellation and reset/factor/
  generation boundaries. Actual child-process abrupt exit preserves its complete
  prefix and leaves a lock; verified manual test cleanup permits a fresh session.
  Startup history failure precedes Source.Start; real oversized-row writer
  failure leaves source updates and HTTP alive and returns error on shutdown.
- Final automated verification, all exit 0:
  - go test ./... -count=1
  - go test -race ./... -count=1
  - make lint (go vet ./... and gofmt check)
  - make test-web (26 tests)
  - make build-dashboard build-obdprobe build-fuelanalyze build
  - built dashboard/fuelanalyze/obdprobe --help; node --check of new modules
  - Go AST function-length check: no function exceeds 100 lines; goimports is
    unavailable, imports were grouped manually and formatted with gofmt.
- Updated README, FUEL_COMPUTER, NEXT_CONNECTION and the approved spec/plan.
  Hardware-only work remaining: short parked M6.1 recording, controlled ignition/
  BLE recovery, IAT validation/cold-start evidence, endurance, Android and actual
  full-to-full fuel calibration. No exact consumption/ECU-model claim is made.

- Final built-binary smoke passed: 25s speed-density simulator capture at 250ms,
  two SSE readers, clean SIGINT, private 0600 history, CLI JSON, restart preserving
  reset_at/totals and two saved sessions, then history-only dashboard with fuel
  off and no BLE. Isolated nonpersistent WebKit rendered all six charts at
  1280x800 and 560 CSS pixels with no JS errors or horizontal overflow. The
  longer moving session was explicitly selected. Artifacts (synthetic only):
  /private/tmp/lanos-m6-1-review-2nl2_p06 (report.json and screenshots).
  A temporary Swift smoke-harness compile error was fixed before this successful
  final run; application code was not implicated. Android itself was not tested.
- git diff --check passed; no changes to go.mod/go.sum, internal/probe,
  internal/elm327 or fuel state schema. Owner test.py/test2.py remain untracked
  and excluded. Delivery stays on feat/fuel-analytics; main is not modified.


### 2026-10-29 — Parked running-engine Check Engine read, owner authorized

- Owner reported a newly illuminated Check Engine, engine running and vehicle
  stopped; clarified steady light and smooth engine operation. No other OBD
  process/listener was found before opening the single BLE connection.
- Ran existing built native Go probe, without changing application code:
  ./bin/obdprobe --transport=ble --ble-name=IOS-Vlink --retries=0 --json
  --output=diagnostics/check-engine-20261029-001.json
- Completed at approximately 13:10 Europe/Moscow: status=complete, one attempt,
  all 12 commands ok, no warnings, exit 0 and normal helper cleanup.
  ELM327 v2.3, auto protocol A5; RPM 874.25, speed 0 km/h, coolant 94°C.
  ATRV 13.1V is adapter supply, not a direct generator-terminal measurement.
- MIL is ON: 0101 -> 41 01 80 07 E1 A1. Low seven bits of A report zero stored
  DTCs. Mode 03 -> 43 00 00 00 00 00 00, decoded as an empty stored-code list.
  The raw responses agree with the decoder; this is not evidence of a healthy
  vehicle or an established fault cause. No specific component is diagnosed.
- Preserved private report (0600) at the ignored path above. No clear/write,
  ignition change, state reset, proprietary query or repeated polling occurred.
  Existing allowlist supports Mode 01/03 only; pending Mode 07 and permanent
  Mode 0A were NOT queried. Next diagnostic step is pending-code reading with
  suitable read-only support and, if needed, ECU-specific diagnostic equipment.
  No new M6.1/Android/fuel-accuracy acceptance is inferred from this probe.


### 2026-10-29 — Explicit pending-code read implemented and physically executed

- Owner said «читаем» after Mode 07 was identified as the next read-only step.
  Worked in the same checkout on feat/pending-dtc, based on 27fcf58. The previous
  hardware log in STATUS was retained. Owner test.py/test2.py were untouched.
- Added finite-only --read-pending to obdprobe: literal 07 allowlisted, strict
  response 47 decoding shares stored-code pair/padding validation, optional
  pending_dtcs is separate from stored_dtcs with raw/time/source/status. Retry
  reset/invalidation cannot expose prior codes as current. Default probes,
  dashboard and recording schedules remain unchanged; Record/Stream and list
  mode reject the option before opening hardware. Mode 04/08/0A/VIN/arbitrary
  commands remain blocked; no Go/dependency/fuel-state change.
- Tests first failed on the missing pending API, then passed: synthetic codes,
  empty/malformed/wrong-mode/negative/NO DATA replies, opt-in/default audit,
  restart/staleness, finite-only flags, and separate human/JSON output. Added
  exact physical MIL/03/07 fixture; nonzero P0133 test codes are synthetic only.
- Offline checks passed: go test ./... -count=1; go test -race ./internal/elm327
  ./internal/probe ./cmd/obdprobe -count=1; make lint; go build -o bin/obdprobe
  ./cmd/obdprobe. Focused tests were repeated after adding the physical fixture
  and human/JSON output checks; no vehicle access is needed for these tests.
- After confirming no concurrent OBD process, ran one native BLE probe:
  ./bin/obdprobe --transport=ble --ble-name=IOS-Vlink --retries=0 --read-pending
  --output=diagnostics/check-engine-pending-20261029-001.json
  Completed at 13:16:15 Europe/Moscow, status=complete, one attempt, all 13
  commands ok, no warnings, exit 0 and normal connection/helper close.
- Actual replies: 0101 -> 41 01 80 07 E1 A1 (MIL on, stored count 0);
  03 -> 43 00 00 00 00 00 00; 07 -> 47 00 00 00 00 00 00.
  Both code lists available and empty. RPM=883, coolant=94°C, speed=0 km/h;
  ATRV=13.2V is adapter supply. Report preserved privately in ignored diagnostics/.
- No fault cause/component diagnosis follows from these empty generic lists.
  Next useful step: suitable ECU-specific GM/Daewoo diagnostic equipment and
  vehicle inspection; ECU model remains unconfirmed. Mode 0A/permanent codes,
  freeze frames, proprietary reads and code clearing were not performed.
  No change to ignition/engine state was requested or made by the agent.


### 2026-11-03 — M3.3.1 completed offline

- Owner saved M3.3 in docs/PLAN.md and requested M3.3.1–M3.3.4 only. Created
  feat/mil-diagnostics from d95da3a (contains M4/M6/M6.1/Mode07). Preserved the
  owner's PLAN addition and untracked scripts; no worktree or hardware access.
- Read AGENTS, PROJECT, PLAN, STATUS, capabilities, FUEL_COMPUTER and existing
  probe/live/fuel/history code. Analyzed the exact requested part under
  data/fuel-history/live, read-only with Python standard-library JSON streaming.
  1299 valid rows, no JSON errors, correct session_end, SHA-256 and Moscow times
  recorded in docs/diagnostics/FIRST_TRIP_ANALYSIS.md.
- The file is 2026-10-29 11:44–12:06, 22m7s, with one failed startup generation
  and a later successful generation; 4989 commands/17 errors/two gap counters.
  Constant IAT=20 across 149 distinct capture timestamps, ECT 6..89, RPM 0..3248.5,
  speed 0..78, MAP 25..102, trims/O2 and adapter supply ranges are documented.
- No MIL/DTC/readiness/fuel-system status or load existed in the saved snapshot;
  original MIL onset and its cause cannot be reconstructed. This record does
  not contain the old -40→20 transition. Kept hypotheses separate from evidence.
- Verification: Python streaming analysis + integrity/unique-timestamp/TTL checks
  passed; git diff --check. No application changes or hardware tests in this step.
  Next: M3.3.2 read-only readiness/permanent/freeze-frame/headers with fixtures.

### 2026-11-03 — M3.3.2 read-only diagnostic extension, offline

- Added 0101 spark/compression readiness and finite-only obdprobe options
  --read-permanent, --read-freeze-frame and --headers. Mode0A lists, frame00
  context and stored/pending lists have independent status/raw/time fields.
  Mode02 reads its bitmap before advertised first-block context PIDs only.
- Negative replies distinguish explicit NRC11/12 unsupported, unavailable and
  malformed responses. Strict service/frame/length checks include raw-only
  freeze PID03. MIL ON with empty standard lists explicitly remains unresolved.
- ATH1 accepts only validated physical three-byte inline-length KWP frames;
  responder metadata and optional validated checksum are retained. Unknown
  layouts fail explicitly. Completed finite scans restore ATH0; cancellation or
  an unsynchronized timeout sends no cleanup. See MIL_DIAGNOSTICS.md.
- Existing default command order and stream options remain unchanged. ECU
  write/clear, arbitrary requests, VIN and proprietary commands stay blocked.
  No dependencies, Go version, fuel formulas/state or hardware access changed.
- Verification: go test ./... -count=1; focused tests and race tests for
  internal/elm327, internal/probe, cmd/obdprobe; make lint; go build -o
  bin/obdprobe ./cmd/obdprobe; git diff --check. All passed. Independent spec
  review found a PID03 length gap; regression failed first, fix and re-review
  passed. No physical Mode02/0A/ATH1 compatibility is claimed.
- Next: M3.3.3 shared-session MIL monitoring, freshness/loss events and 60s ring.

### 2026-11-03 — M3.3.3 MIL monitor and shared-session scheduling, offline

- Added MILMonitor: initial MIL_ALREADY_ON, confirmed MIL_ON/MIL_OFF, UNKNOWN
  on expiry/loss, and separate observations after gaps. New response timestamps
  establish continuity; duplicates or old MIL cannot become a new transition.
  Events retain count/raw/readiness/host timestamps and transition bounds.
- Source publishes every update to one observer registered before Start, so
  coalesced Read and browser count cannot lose MIL transitions. Additive snapshot
  fields retain independent MIL TTL, read-only diagnostic evidence and last poll
  latency/errors; cloning owns all pointers/maps. Load is exposed from existing
  0104 polling. Stored freeze-frame values never enter live/fuel signals.
- Config.MILInterval=0 preserves defaults; enabled Stream accepts 1s..60s. Core
  reads precede a due-MIL slot and at most one ordinary/diagnostic slot. Separate
  slots prevent starvation at 1s; diagnostics alternate with due ordinary work.
  Queue: 03/07/0A/020000 then advertised frame00 context. Unsupported requests
  latch per generation. No new connection, commands remain serialized/read-only.
- 60s ring samples at most once per second plus forced boundaries, hard cap256.
  One boundary predecessor handles fractional event timing; missing start or
  capacity-evicted pre-context is explicit. Independent spec and quality review
  passed after red-green fixes for the boundary and duplicate loss events.
- Verification: go test ./... -count=1, focused monitor/source/telemetry tests,
  focused race tests, make lint and git diff --check passed. Concurrent stress
  reproduced old 1–5ms recovery-fixture flakes; three tests now inject one
  synthetic timeout while healthy commands retain 1s deadlines. Recovery/prompt
  assertions remain; probe race stress passed five runs at parallelism64 and
  focused recovery tests passed 50 runs. Production timeouts are unchanged.
- No hardware, fuel formulas/state, dependencies or Go version changes.
  Next: M3.3.4 capture manager, private bounded JSONL and dashboard CLI wiring.

### 2026-11-03 — M3.3.4 diagnostic snapshots and JSONL completed offline

- Added opt-in dashboard --mil-record-dir and --mil-interval (default5s,
  target1..60s). Early validation rejects incomplete/mixed configuration.
  One capture observer attaches before Source.Start; no second transport or
  browser-dependent accounting. API/SSE expose mil, ecu_readings, poll and
  mil_recording; the dedicated diagnostic screen remains M3.3.6.
- ON/MIL_ALREADY_ON/ON_AFTER_GAP preserve retained60s pre/30s post context.
  Nearby triggers share an incident, deadline bounded at first+120s. Every
  transition/loss is saved even outside an incident; late queued diagnostics
  have independent rows without guessed association. Initial/missing pre,
  capacity eviction, stale data, timer gaps and early shutdown stay explicit.
- Frames retain raw telemetry, signal ages/effective health, load/trims/O2,
  adapter supply, connectivity/generation/counters, last latency/error, read-only
  diagnostic evidence and existing Fuel.Diagnostic with its own input times.
  The getter never advances fuel accounting; a real Computer regression verifies
  unchanged totals/state. Stored freeze readings never become live fuel inputs.
- New milstore: bounded256 asynchronous queue,1MiB row cap, default8x16MiB managed
  parts, strict source-bound manifest and retention flag. New directories0700,
  files0600, exclusive lock, no overwrite or foreign-file removal, VIN redaction,
  fsync5s/rotation/close. Symlink paths rejected; orphan/corrupt records preserved
  with actionable failure. Missing footer/aborted parts remain incomplete.
  Runtime failure appears in API/stderr/Close while telemetry/fuel continue.
- Tests cover full synthetic60+30s through real JSONL, initialON/partial close,
  overlaps/hard deadline, late replies, no fabricated past frames after delayed
  ticks, read-only fuel context, queue/disk/lock/crash/retention/privacy, fast
  OFF/ON/OFF before Read, and two SSE browsers sharing one source. MIL-specific
  server failure test confirms visible API failure plus continuing fuel/telemetry
  and returned shutdown error. Independent spec/quality reviews passed.
- Final verification passed: go test ./... -count=1; go test -race ./... -count=1;
  make lint (includes go vet ./...); make test-web (26 passed); make
  build-dashboard build-obdprobe build-fuelanalyze build (including native BLE
  helper); git diff --check. All changed Go functions are <=100 lines; goimports
  unavailable, imports grouped manually and gofmt clean. No dependency/Go version
  changes, ECU writes, fuel-state/formula changes or hardware access.
- Built-binary synthetic smoke passed: two simulator launches, each with two
  SSE clients, recording status, private permissions, clean SIGINT/exit0,
  exclusive-lock removal, two distinct persisted sessions across restart and
  invalid flags rejected before startup. Simulator intentionally has no MIL;
  complete ON/OFF capture is covered by the synthetic Go integration tests.
  Artifacts: /private/tmp/lanos-mil-smoke-2nno8s5m/report.json.
- Updated README, PLAN, MIL_DIAGNOSTICS and NEXT_CONNECTION with exact next-run
  command, interpretation/retention/lock guidance and unchanged fuel-state path.
  Private data/mil-events is ignored; owner scripts remain excluded. Delivery on
  feat/mil-diagnostics; no main merge. No new hardware/Android/fuel-accuracy claim.
  Next: separately authorized M3.3.7 parked physical acceptance and/or M3.3.5
  offline correlation, then M3.3.6 diagnostic screen; these remain unimplemented.

### 2026-11-06 — Two owner-recorded trips: missing instantaneous fuel analyzed

- Owner reported no fuel display during the first drive and a late appearance
  during the second. Read fuel-history live parts4/5 and matching MIL parts1/2
  offline, without opening hardware or changing application/fuel-state settings.
- JSON/sequence/closed-footer/hash checks passed. Records cover 09:22:48–09:35:19
  and 09:52:43–09:57:42 Europe/Moscow; about1.6704/1.6713 recorded km. Full evidence
  and limitations: diagnostics/2026-11-06-fuel-availability.md.
- All blocked moving snapshots have fresh available IAT<=-30 and the exact
  reason iat_unavailable_or_out_of_range. First drive:175/175 moving snapshots
  blocked at -40. A separate53s standstill interval at -6/-7 did produce L/h.
  Second drive:101/169 moving snapshots blocked; IAT becomes+8 at09:55:51.449
  and the fuel estimate appears at that exact timestamp, then follows+8..+10.
- Confirmed experimental IAT opt-in was enabled after model selection. IAT TTL20s
  and capture intervals~8.7s show the blocked moving samples were fresh. Existing
  estimator/browser behavior explains the dash; no threshold/default-temperature
  substitution, model/factor change or full-trip fuel reconstruction performed.
- Existing offline fuelanalyze agrees: time coverage8.06%/35.10%, covered fuel
  0.02306/0.08506L; these are partial records, not complete-trip consumption.
  Both MIL histories start OFF with empty03; no ON/Mode02/0A physical acceptance.
- Separate follow-up observed: MIL UNKNOWN/OFF_AFTER_GAP with small/negative ages
  during otherwise live telemetry warrants timestamp-ordering regression work.
  These events are not sufficient evidence of BLE loss or the cause of fuel gaps.
- Verification: focused go test ./internal/fuel -count=1 and five web/fuel tests;
  JSON/sequence/hash/all-blocked-input assertions and both offline CLI reports.
  No application code changes; private parts and owner scripts remain untouched.
  Next: raw010F evidence and independent IAT comparison under a separately
  authorized connection; sensor/wiring/ECU cause remains unconfirmed. This
  targeted investigation does not implement the deferred M3.3.5 analyzer.

### 2026-11-06 — MIL false UNKNOWN timestamp ordering reproduced and fixed

- Owner confirms the lamp did not illuminate during either short trip. All181
  distinct successful saved0101 observations report OFF/count0; Mode03 is empty
  in each session. No ON/automatic Mode02/0A snapshot acceptance is inferred.
- Found11 intermediate stale UNKNOWN events and two final NO DATA events. One
  event at09:31:11.433739 evaluates a response captured at09:31:11.434998, about
  1.26ms later than the saved timer timestamp. Freshness correctly rejects future
  evidence, but the caller passed an older tick after a newer publication.
- Deterministic synthetic tests first failed: old ticks invalidated new MIL or
  core-only evidence and changed a confirmed ON into ON_AFTER_GAP. Capture now
  samples live observation time under its lock and ignores ticks older than the
  latest source observation. No TTL relaxation, timestamp substitution in raw
  inputs, command/transport changes or fuel model/state changes.
- Regression tests preserve genuine source silence, independent MIL expiry,
  truly future input and NO DATA -> UNKNOWN -> OFF_AFTER_GAP. Disabling the guard
  again reproduced all three stale-tick failures; restored guard passes.
- Verification passed: go test ./... -count=1; go test -race ./internal/mil
  ./internal/live ./internal/server ./internal/fuel -count=1; make lint (vet and
  gofmt); git diff --check. Earlier fuel-analysis checks and five web/fuel tests
  also passed. Historical parts and owner scripts remain untouched.
- Updated the trip analysis and MIL documentation. Rebuild dashboard before the
  next run; normal command/flags stay the same. No adapter connection this turn.
  Next: observe event history in another owner-authorized run and separately
  investigate raw IAT. Historical UNKNOWN rows are preserved as original evidence.

### 2026-11-07 — Three owner-recorded sessions: P0420 and fuel coverage

- Reviewed fuel parts6–9 (parts7/8 are one rotated session) and MIL parts3–5
  offline. No vehicle connection, ECU commands/clearing, application/settings
  changes or modifications of private recordings/owner scripts.
- All today's JSON/version/session/sequence/footer checks passed, including
  rotation continuity. Saved SHA-256 and detailed evidence in
  diagnostics/2026-11-07-trip-review.md. Fuel retention flag concerns an older
  retired part; all today's sessions are present. MIL retention is untruncated.
- At16:54:18.224874 MIL reports ON/count1 after OFF at16:54:12.027771. Automatic
  03/07 both return P0420; frame00 cause also P0420, with RPM925.25, speed0,
  ECT85, MAP44, STFT-1.5625%, LTFT-11.71875%. Mode0A returns7F0A11 unsupported.
  This is observed owner-recorded physical-response evidence for those reads,
  not completion of the full M3.3.7 checklist or confirmation of today's lamp.
- Incident retains53pre/31post frames; pre_window_retained=true,
  post_window_retained=false, had_gaps=true. Later frame00 replies are saved
  independently. Last confirmed ON16:55:11; subsequent UNKNOWN is expired
  evidence, not proof that MIL switched off.
- Existing fuelanalyze exits0 for all three sessions:3.0342/20.9959/8.6779km,
  covered estimated0.22486/2.13377/0.67248L, covered averages12.119/10.267/8.240
  L/100km, time coverage47.23/97.30/75.29%. All incomplete and experimental;
  no full-trip reconstruction or real consumption-accuracy claim.
- First/second sessions still contain-40 IAT. Third-session numeric IAT is
  +10..+29, but all30 IAT-blocked snapshots exceed20sTTL (age20.005..28.966s).
  RPM/MAP/source expiry also affects fuel. Saved poll latency worsens before
  the MIL event. Owner subsequently clarified that an OS process consumed CPU
  and the entire host became severely unresponsive during this last recording.
  Host overload supplies context for delayed polls/expiry; no saved CPU profile
  quantifies its contribution. Do not infer an adapter/ECU defect from these
  gaps. P0420 evidence and earlier-40IAT observations remain valid observations.
- First two MIL sessions contain no UNKNOWN. Third has five intermediate
  UNKNOWN/OFF_AFTER_GAP pairs plus final UNKNOWN, nonnegative MIL ages and
  evidence of real global-source expiry. Yesterday's negative-age case is not
  reproduced; this does not prove every freshness/reconnect scenario accepted.
- Verification: offline CLI reports and targeted JSON/sequence/footer/hash/IAT
  assertions; documentation diff check. Go/web suites not rerun: no code changes.
  Missing .codex/START_HERE.md remains absent; root AGENTS and project docs read.
- Next: preserve codes, confirm physical lamp/current DTC on a separately
  authorized connection; independently investigate IAT and exhaust/O2/fueling
  before any catalyst conclusion. Sparse O2 data cannot compare waveforms;
  IAT-to-P0420 causality is unproven. M3.3.5/6 remain unimplemented.

### 2026-11-07 — M3.3.5 design prepared

- Owner approved proceeding with M3.3.5. Created feat/mil-analysis from
  feat/mil-diagnostics, preserving the existing trip-review documentation.
- Inspected MIL envelope/manifest, frame applicability, incident closure and
  independent late diagnostic rows. Proposed reusable offline milanalysis plus
  milanalyze text/JSON CLI, without changing live collection or joining fuel
  sessions by guessed identifiers.
- Concrete design: superpowers/specs/2026-11-07-mil-analysis-design.md. Includes
  bounded reader, integrity/coverage, evidence provenance, heuristic thresholds,
  insufficient-data outcomes and regression/real-recording checks. Spec reviewed
  for scope, ambiguity and placeholders; documentation whitespace checks pass.
- Implementation awaits review of the written specification required by the
  brainstorming skill; no analyzer code or new hardware work in this entry.

### 2026-11-07 — M3.3.5 implemented: offline MIL evidence and anomaly reports

- Owner approved the written specification and implementation. Delivery stays
  on feat/mil-analysis, based on feat/mil-diagnostics. No dependency/Go version,
  live schedule/transport, fuel model/state or saved format changes. No hardware
  connection, ECU command or clearing; owner scripts remain excluded.
- Added milstore.Read: bounded read-only streaming with private regular-file
  manifest checks, session selection, cross-rotation sequence/elapsed/closure
  validation, typed payload checks and explicit corruption/retention/boundary
  issues. Tests compare archive hashes, mtimes, directory contents and no lock.
- Added milanalysis with bounded per-incident context, scoped MIL history and
  separate stored/pending/permanent/frame00 evidence. Unassociated late replies
  remain separate; contradictory successful details or absent decoded lists
  become issues, never empty success or confirmed P0420. Error footer reason
  and text are retained; closed files can still describe failed recording.
- Freshness/unit/support-aware heuristics cover IAT, MAP context, trims, O2,
  ECT, adapter supply and quality. Capture/generation dedup and continuity
  boundaries prevent repeated snapshots or gaps becoming new measurements.
  Every rule has an explicit insufficient-data outcome. Findings retain full
  observation bounds/counts, up to8 evidence/counterexample samples and missing
  data/next checks; thresholds are observational heuristics, not diagnoses.
- Added milanalyze text/JSON CLI and build target. Required --input; optional
  session and session-scoped incident filter; errors go to stderr. Text uses
  Moscow time, JSON is deterministic. No source/transport packages appear in
  go list -deps ./cmd/milanalyze. Context timeout30s and cancellation supported.
- TDD reproduced missing feature behavior and review regressions: stale/future/
  unsupported/wrong-unit input, cached values across gaps and A->B->A generation,
  sparse vs dense O2, bad successful DTC details, unavailable polls, failed
  footer, missing rule checks, bounded evidence and explicit frame truncation.
  Reader and analyzer passed independent spec-compliance and quality reviews;
  reported issues were reproduced, corrected and rechecked.
- Fresh verification passed: go test ./... -count=1; go test -race ./... -count=1;
  make lint (go vet/gofmt); make test-web (26 tests); make build-milanalyze and
  final go build -o bin/milanalyze ./cmd/milanalyze; git diff --check.
  Build target initially needed sandbox approval for the Go cache, then passed.
- Offline binary acceptance on saved session7f688937053f9f030d0f6bbfc3477a07:
  13events,14diagnostics,84unique context frames, P0420/empty initial03,
  unsupported0A, incomplete post, insufficient O2, final UNKNOWN and4independent
  late frame00 replies preserved. Repeated JSON byte-identical; input hashes
  and mtimes unchanged. All6currently retained MIL sessions read without issues;
  a sixth closed part appeared externally during development, without changing
  the earlier three-trip manual analysis. Artifacts:
  /private/tmp/lanos-milanalyze-acceptance-a8lwr_1n/report.txt and report.json.
- README, PLAN, MIL_DIAGNOSTICS and NEXT_CONNECTION include offline commands and
  limits. Sensor cause, true fuel accuracy, catalyst condition, CPU vs BLE delay
  attribution and Android are not established. M3.3.6 and full M3.3.7 remain next.

### 2026-11-07 — M3.3.6 diagnostics screen design prepared

- Owner requested continuing after M3.3.5. Created feat/mil-dashboard from
  1cb2909, preserving owner scripts. Read existing SSE/freshness, history UI,
  server/CLI wiring and completed analyzer contracts. No product-code changes.
- Proposed a separate diagnostics page in the existing style: current MIL and
  timestamped DTC replies above saved sessions/events/findings, with report and
  incident-context export. Read-only --mil-history-dir permits viewing saved
  LIVE data with a SIM server and no adapter connection; labels stay separate.
- Design: superpowers/specs/2026-11-07-mil-dashboard-design.md. Includes bounded
  HTTP report/export, one analysis at a time, no background archive scan per SSE,
  independent live freshness, partial evidence, safe DOM and unchanged ECU/fuel.
- Builder check identified one contract ambiguity, resolved as an implementation
  choice: recoverable reader issues return200/incomplete with surviving evidence;
  fatal manifest/open/rotation errors return409. No owner-only open questions.
- Whitespace/placeholder checks passed; application tests not rerun for this
  design-only change. Written design awaits approval before implementation.
  M3.3.6 remains unimplemented; no hardware, dependency or format changes.

### 2026-11-07 — M3.3.6 implemented: dashboard diagnostics and read-only history

- Owner approved the written design. Implemented on feat/mil-dashboard from
  feat/mil-analysis; no merge to main. No vehicle connection, new ECU commands,
  polling changes, dependencies/Go version, fuel model/state or archive format
  changes. Owner scripts and private raw recordings remain outside commits.
- Added /diagnostics.html with current MIL ON/OFF/UNKNOWN, validated 0101 count,
  independent MIL/source TTL and transport freshness. Monotonic watchdog ages
  evidence without SSE; source/generation changes and future/invalid replies
  cannot silently display cached MIL as live. Last success is explicitly scoped
  to this page opening. Stored/pending/permanent replies have their own times,
  distinct empty/unsupported/unavailable/invalid/no-query states, and historical
  labels. Unexplained ON retains the required warning, never “no errors”.
- Read-only milstore.List and GET /api/mil/history/{sessions,report,incident}
  provide validated catalog, existing milanalysis report and exact incident
  selection with envelope/file/line provenance. Independent late replies remain
  outside incident exports. Recoverable corruption retains evidence as incomplete;
  fatal archive errors are sanitized. Known query keys/IDs only, one heavy request
  per server without queue or telemetry lock, 8s context and 8MiB buffered output.
- --mil-history-dir enables reading without Capture/writer/polling or directory
  creation; --mil-record-dir implies the same history directory. Fuel directories
  cannot overlap. UI separates current SIM from archive LIVE. Opening loads catalog
  and latest report once; subsequent archive work is manual. Browser uses 10s
  timeout, cancellation and request epochs; selection/error removes old reports
  and export links. Saved text uses DOM/textContent. Events, DTC/freeze replies,
  partial windows, findings/evidence/limits and insufficient_data are accessible.
- TDD and independent task review covered HTTP/flags/immutability/concurrency,
  stale/future/source/generation/DTC cases, request races/limits and safe DOM.
  Review found pagehide stopped Feed/watchdog without pageshow recovery. Node
  and actual Chrome-renderer lifecycle regression reproduced ON instead of
  UNKNOWN. Fixed immediate invalidation on hide and one bounded Feed/watchdog
  restart on persisted show, without archive rescan; regression now passes.
- Verification passed: go test ./... -count=1; go test -race ./... -count=1;
  make lint (vet/gofmt); make test-web (50/50); make build-dashboard (including
  native BLE helper); make build-milanalyze; git diff --check. Full Go/race/lint
  ran before the browser-only lifecycle fix; final web tests and dashboard build
  ran afterward. Go-cache sandbox denials were rerun with required approval.
- Chrome156 offline acceptance used the built SIM/fuel-off server, a synthetic
  LIVE archive and saved session7f688937053f9f030d0f6bbfc3477a07. Verified P0420,
  13 events/14 diagnostics, incomplete incident export excluding independent
  replies, navigation/assets, SIM/LIVE labels, silent-SSE expiry, persisted
  lifecycle events, disabled/empty/missing history and clean runtime. Input
  hashes, mtimes and listings unchanged. Visually inspected 1280×800/390×844
  pages, event/rule details and expanded raw DTC; no horizontal page overflow.
  QA artifacts: /private/tmp/lanos-mil-dashboard-qa.1hSJ9G (local only).
- README, PLAN, MIL_DIAGNOSTICS and NEXT_CONNECTION updated. Offline command:
  ./bin/dashboard --source=sim --fuel=off --mil-history-dir=data/mil-events/live
  then /diagnostics.html. Existing live/fuel launch remains documented. No
  Android-device acceptance, physical M3.3.7 or new compatibility claim; actual
  browser Back/Forward cache admission was not tested, only persisted lifecycle
  handling. Next: separately authorized parked-car M3.3.7, physical lamp/current
  codes and independent IAT investigation; do not clear DTC or infer catalyst
  condition from sparse O2 evidence.

### 2026-11-11 — Ignition-cycle and Mac sleep recovery investigation

- Reviewed today's fuel parts c/d and MIL8/9 offline. Both sessions/footers and
  sequences validate; original hashes/mtimes/sizes unchanged. No adapter access,
  application/fuel-state changes, ECU commands or clearing. Owner scripts untouched.
- First session: initial ECU standby recovers09:01:30. After RPM0 at09:03:41,
  standby starts09:03:44; ATI stays successful but sampled0100 remains unavailable.
  Owner confirms the engine was running again and dashboard restart was needed.
  Restart09:05:17 restores data09:05:30. waitECU only repeats ATI/0100 without
  escalating to reinitialization. Exact ECU/ELM failure cause is not established.
- Second session: engine reaches RPM0 and ECU standby09:19:01. Host wall/elapsed
  divergence311.667s matches owner-confirmed lid close/sleep. ATI times out after
  wake; synchronization attempt ends09:24:42 with native Write and notification
  unsubscribe timeouts, helper exit1. Source permanently reports restart-required
  until09:31:25; counters remain3177commands/17errors/2gaps/1recovery. Another
  128.374s clock divergence is observed, without separate owner attribution.
- Traced terminal outcome through nativeConnection.Close, finishConnection,
  parent BLE error and Stream transient classification. Generic cleanup failures
  intentionally terminate today; increasing retries alone does not fix this path.
  Required repair scopes: bounded ECU-standby escalation and explicit sleep/native
  timeout recovery after fully reaping the helper, preserving fatal error evidence,
  prompt synchronization, freshness, one source and persistent fuel totals.
- Both sessions retain alreadyON/count1 with stored/pending/freeze P0420, Mode0A
  unsupported, then UNKNOWN on lost source evidence; no confirmed lamp-off/new
  OFF-to-ON. Physical lamp state not checked. Second drive8.8909km, covered model
  estimate0.84787L/9.5348L100km, coverage71.47%; not measured consumption accuracy.
- Four offline fuelanalyze/milanalyze reports exit0; focused existing probe/BLE/
  helper recovery and cleanup tests pass. No full Go/race/web rerun for this
  analysis/documentation-only change. Missing .codex/START_HERE.md remains absent.
  Detailed timeline, evidence, hashes and next regressions:
  diagnostics/2026-11-11-reconnect-review.md. No application repair/hardware
  acceptance claimed; next physical checks require separate parked-car approval.

### 2026-11-11 — Ignition/host-sleep recovery repairs implemented offline

- Owner authorized both repair scopes from the investigation. Created
  fix/ignition-sleep-recovery from29c2106, preserving owner scripts. No adapter
  connection, dependency/Go version, new ECU command, fuel model/state/reset or
  archive-format changes. Design and implementation plan are under superpowers/.
- Continuous Stream now reinitializes the synchronized existing ECU session after
  3 negative standby0100 checks with valid ATI, each separated by10s. Emits
  ecu_reinitialize recovery and starts a new segment; full PID/DTC scan still
  requires a valid bitmap. Initial availability resumes sooner; persistent absence
  stays sparse/cancelable. Finite Run/Record loops remain bounded and unchanged.
- Added blewire.ErrTimeout and timeout IPC code. Exact private/plain timeout
  messages from pinned native Write and subscribe/unsubscribe calls are mapped
  only at those operation boundaries; unknown/wrapped/permission causes are not
  guessed from substrings. Native documents checked before targeted library source.
- Helper failed writes now use the final error after cleanup, avoiding an early
  untyped ack race. Recognized transient primary error plus timeout-only cleanup
  can exit successfully with both causes in the final event. Parent retains typed
  timeout, verifies/reaps the old child before Stream reconnects. Mixed/unknown
  cleanup, normal-shutdown cleanup timeout, output failure and abnormal helper
  exit remain errors. Existing prompt recovery still blocks writes until safe.
- TDD RED reproduced indefinite ECU wait, untyped native timeout, failed-ack race,
  compound timeout terminal stop and Stream failing to retry with retries0.
  Additional RED caught overly broad acceptance of mixed cleanup; narrowed to
  timeout-only leaves. Tests cover reinitialization without DTC storms, cancellation,
  operation-specific matching, late final timeout after command deadline, output
  errors, clean/failed child exit, close-before-reopen and joined fatal causes.
- Fresh checks pass: go test ./... -count=1; go test -race ./... -count=1;
  make lint; make test-web(50/50); make build-dashboard build-obdprobe
  build-milanalyze. Native helper compiled on macOS with CGO. Sandbox-denied Go
  cache checks were rerun with required approval. Formatting uses gofmt;
  goimports is unavailable on PATH; no tool/dependency installation was needed.
- README, PLAN and NEXT_CONNECTION describe new escalation and narrow timeout
  exception, plus separate awake ignition-cycle and host sleep/wake checks.
  Physical recovery, adapter wake, delivery of outstanding prompt and Android
  remain unverified. No blind ATZ/prompt bypass. Independent whole-branch review
  approved the offline implementation without findings. Built SIM/fuel-off HTTP
  smoke passed snapshot/three pages and graceful SIGTERM exit0. Four recorded
  source hashes/mtimes/sizes remain unchanged. Original recorded evidence remains
  separate from synthetic regressions; delivery stays on the feature branch.
