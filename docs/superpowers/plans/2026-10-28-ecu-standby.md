# ECU standby implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement inline,
> task by task, with test-driven-development. No delegated workers.

**Goal:** Keep LIVE waiting for an unavailable ECU/adapter and resume automatically.

**Architecture:** A continuous-only supervisor in probe retains session synchronization
and bounded retry bursts. Typed standby events drive existing connecting/message
snapshots; ECU responses alone establish ECU freshness. Finite Run/Record are preserved.

**Tech Stack:** Existing Go 1.27 standard library, native BLE helper and Node tests;
no dependency changes. Approved spec: ../specs/2026-10-28-ecu-standby-design.md.

## Task 1 — Reproduce continuous-stream failure

Files: internal/probe/standby_test.go, existing record_test.go fixtures.

- [x] Add fixture tests for startup NO DATA/UNABLE TO CONNECT, repeated empty
  rounds, return after ignition on, individual missing PID and zero RPM.
  Use recordingRunner's injected clock/wait, a bounded test context and cancel
  on recovered capabilities. Assert no repeated full scan while waiting.
- [x] Add tests for retries=0 disconnect/open failure, capped burst delays,
  pending prompt across pause/reopen, cancel, fatal configuration/sink/close
  errors. Assert no write precedes recovery. Preserve finite Record regressions.
- [x] Run `go test ./internal/probe -run 'TestStream' -count=1`: observe failure
  before production edits, including absent standby events and exhausted retries.

## Task 2 — Continuous-only supervisor

Files: internal/probe/standby.go, record.go, scan.go; if needed BLE helper
error boundary in internal/blewire/protocol.go, blehelper and ble.

- [x] Dispatch duration=0 to runLive; keep finite recording.run unchanged.
  Extract post-discovery samples from scan so LIVE can stop before DTC/sample
  polling when bitmap unavailable. Detect three failed core rounds only in LIVE.
- [x] Emit typed standby events with ECU/adapter/recovery reason. Wait 10s
  between idle checks; check ATI then 0100 without full scans on negative replies.
  Resume a new segment/discovery only on valid capabilities. Handle no-core
  adapters with sparse 0100 rather than treating ATRV as ECU activity.
- [x] Keep session after timeout for read-only Recover; preserve needSync after
  reopen. Retry only known transient errors; permission/configuration, unknown
  errors, malformed protocol, sink and cleanup errors remain terminal.
  Use bounded 1+Retries bursts and a 10s inter-burst pause; all waits use ctx.
- [x] Run probe/ELM/BLE tests and confirm both new tests and finite regressions.

## Task 3 — Snapshot, UI and fuel boundaries

Files: internal/live/events.go, source.go, standby_test.go;
internal/server/live_test.go; web/model.test.mjs (model.mjs only if needed).

- [x] Add failing tests: empty capabilities and AT activity cannot establish or
  refresh ECU connectivity; standby clears values and retains LIVE; a new
  segment increments generation; actual ECU zero stays valid. Check HTTP/SSE
  stays available for two clients and cancellation joins the single worker.
- [x] Map standby to connecting with explicit Russian ECU/adapter/recovery text;
  permit connected only with decoded ECU evidence. Use ECU timestamps for global
  freshness. Keep current independent signal expiry and terminal error evidence.
- [x] Verify fuel totals/instantaneous expiry and no integration across a lost
  standby event using generation; add focused integration regression if existing
  accounting tests do not cover the real Source transition.
- [x] Run `go test ./internal/live ./internal/server ./internal/fuel -count=1`
  and `make test-web` with waiting-message and numeric-hiding coverage.

## Task 4 — Delivery

Files: cmd/dashboard/flags.go, README.md, docs/NEXT_CONNECTION.md,
docs/PROJECT.md, docs/PLAN.md, docs/STATUS.md and approved spec/this plan.

- [x] Update LIVE help and retry/wait guidance; document terminal failures,
  battery/adapter sleep limitation and next separately agreed parked cycle test.
- [x] Run `go test ./... -count=1`, `go test -race ./...`, `make lint`,
  `make test-web`, both binary build targets and `git diff --check`.
  Verify simulator HTTP/SSE startup and shutdown using isolated temporary state.
- [x] Review diff against spec, record actual evidence/limits in STATUS and
  mark completed tasks. Do not claim a physical standby test.
- [x] Commit explicit project files only, preserving user scripts/private data;
  include required Co-Authored-By trailer. Push fix/ecu-standby and report branch,
  test results and next hardware instructions. Do not merge main.

Implementation and offline checks complete. Delivery is on fix/ecu-standby;
physical ignition-cycle acceptance remains pending as recorded in STATUS.md.
