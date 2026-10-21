# Session handoff / status

## Current state
M0–M2 complete. Working Go backend, deterministic simulator and embedded
responsive dashboard are implemented and verified locally. Only simulator mode
is available. Hardware has not been accessed or tested.

## Next action
M3: implement the read-only probe, ELM framing/parser and PID/DTC decoding with
fixtures. Real adapter/ECU and Android tablet tests require a separate session
and physical hardware; no compatibility claims are made.

## Completed milestones
- M0 — module, package boundaries, commands, README and Makefile.
- M1 — nullable telemetry, freshness, cancellable source contract and simulator.
- M2 — local HTTP/SSE transport, embedded responsive UI and bounded reconnect.

## Decisions and risks
- Local-first, read-only diagnostics.
- MacBook M3 is development host; Android Teclast P85 is target.
- Bluetooth Classic SPP on macOS and Android tablet is not yet verified.
- ECU identity and generic OBD-II PID support not yet verified.

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
