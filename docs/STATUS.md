# Session handoff / status

## Current state
M0–M3 complete. The dashboard remains simulator-only. The new Go read-only
obdprobe provides macOS native BLE and serial, shared ELM framing/recovery,
capability discovery, PID/DTC decoding, bounded reconnect and private JSON export.
Native Go BLE passed a physical baseline on 2026-10-24 with this MacBook/Vgate/
Lanos: RPM 889.25, coolant 52°C, speed 0, ATRV 11.0V, MIL off and no stored DTCs
in the Mode 03 response. All 12 commands succeeded; connection closed normally.
The subsequent native Go supported-PID scan also passed: 27 commands, one
attempt, 18 data PIDs read (14 decoded, four raw_only), no warnings. Bitmaps
BE3EB811/C0000000 were confirmed. MAP/IAT/load/throttle/trims/O2 are now observed;
the latest readings and exact payloads are documented in capabilities.md.
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
New requested work: prepare a native Go trip recorder for fuel-computer research.
Proposed design: docs/superpowers/specs/2026-10-24-trip-recorder-design.md;
awaiting owner review before implementation. No recording command exists yet.

The requested Go utility fix is implemented and physically verified for a BLE
baseline and a supported-PID scan. Build both binaries with make build-obdprobe; use
bin/obdprobe --transport=ble --ble-name=IOS-Vlink --retries=0 with a fresh output
filename. See docs/NEXT_CONNECTION.md for future sessions; reconfirm current
car/ignition state before hardware access. Preserve recovery/capability rules.
Dashboard/live Source integration remains the separate M4 task; no live dashboard
was added. Native Go endurance, ignition-cycle reconnect and stale injection
still need physical tests. PID 03/13/21/22 returned positive raw payloads, but
their semantic decoders remain unimplemented. One O2 sample does not establish
sensor health; MAP/IAT/RPM availability does not validate fuel estimation.
Native Go confirmed 0120=C0000000 and ATRV=11.0V with engine running.
Verify the unexpectedly low adapter voltage with a multimeter
at the battery terminals; it is not a direct generator or ECU voltage measurement.

## Completed milestones
- M0 — module, package boundaries, commands, README and Makefile.
- M1 — nullable telemetry, freshness, cancellable source contract and simulator.
- M2 — local HTTP/SSE transport, embedded responsive UI and bounded reconnect.
- M3 — read-only diagnostic CLI, real-session fixture, discovery and recovery.
- M3 follow-up — native Go BLE transport, private export, baseline physically passed.
- M3 discovery follow-up — native Go supported scan physically passed; 18 data PIDs.

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
