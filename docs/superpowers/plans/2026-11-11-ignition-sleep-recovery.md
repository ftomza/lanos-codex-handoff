# Ignition and sleep recovery implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Recover continuous LIVE from the two owner-observed November11 failures.

**Architecture:** Escalate sparse ECU standby into existing discovery; propagate
narrowly typed native timeouts through helper final events into existing retry.
Preserve sequential ownership, child reaping and ELM prompt synchronization.

**Tech Stack:** Existing Go1.27, standard library, pinned native BLE dependency.

**Spec:** docs/superpowers/specs/2026-11-11-ignition-sleep-recovery-design.md

## Global Constraints

- No new physical connection, ECU commands/clearing, dependencies or Go edits.
- No fuel model/state/reset, archive formats, source defaults or simulator changes.
- Keep10s standby pauses; reinitialize after3 consecutive negative0100 checks.
- Only exact native call-boundary timeout strings are mapped; unknown errors fatal.
- Failed writes use final error after cleanup; successful writes retain ack.
- Cleanup timeout alone on normal shutdown remains fatal; preserve error evidence.
- Reap helper before reopen, retain prompt synchronization and sparse retry bounds.
- Do not inspect/run/modify owner test.py,test2.py,test_mb.py or sensitive paths.
- Private recordings stay unchanged and outside commits.

## Review Focus

- Repeated absent ECU must not trigger full DTC scans or busy retry loops.
- A failed-write ack must not race a typed final timeout into a terminal outcome.
- Timeout joined with permission/unknown cleanup must remain fatal.
- Reconnection cannot overlap the old helper or bypass a pending ELM prompt.
- Finite probes/recordings and source/fuel freshness must retain existing bounds.

### Task 1: ECU standby escalation

**Files:** internal/probe/standby.go; new internal/probe/standby_reinit_test.go.

**Interfaces:** existing recording.waitECU/context and runLive segment restart;
produce recovery event phase ecu_reinitialize, no public API changes.

- [x] Write test TestStreamStandbyReinitializesUnresponsiveECUSession: real
  Runner/ELM fixture stays unavailable until second ATSP0, valid identity,3
  sparse negative checks; assert capabilities resume on same transport/new
  generation, no prior DTC scan and cancellation closes cleanly.
- [x] Write repeated-absence/cancel regression: bounded10s pauses,3 checks per
  initialization, no DTC readings, no extra transport open or writes after cancel.
- [x] Run go test ./internal/probe -run 'TestStreamStandbyReinit' -count=1;
  expect RED due to missing reinitialization/resume.
- [x] Implement3-negative escalation through existing new-segment initialization.
- [x] Run go test ./internal/probe ./internal/live ./internal/fuel -count=1;
  expect all pass. Commit only Task1 files plus approved design/plan/investigation.

### Task 2: Native timeout typing, helper completion and parent retry

**Files:** internal/blewire/protocol.go; new internal/blehelper/native_errors.go
and native_errors_test.go; native_darwin.go, finish.go, serve.go, serve_test.go;
internal/ble/process.go, process_test.go; internal/probe/standby.go,
standby_reinit_test.go.

**Interfaces:** blewire.ErrTimeout with code timeout; helper final error retains
native primary/cleanup text; parent Device operations return typed errors after
final event, Close verifies completion; Stream recognizes new sentinel.

- [x] Write failing table tests for exact operation-specific native mapping,
  unknown/permission/wrapped lookalikes, compound timeout cleanup, normal-shutdown
  cleanup timeout, output/unknown cleanup failure; use real Serve/output stream.
- [x] Write parent subprocess scenario final timeout: assert Write error typed,
  text retained, Close reaps successful child; nonzero exit remains fatal.
- [x] Write Stream retries0 regression using transient timeout then healthy
  transport with pending prompt; assert closure before reopen,10s idle and
  subsequent capabilities. Add joined unknown cause remains terminal.
- [x] Run focused new tests; observe expected RED before product implementation.
- [x] Implement operation-boundary normalization, combined transient policy,
  no failed-write ack, final timeout mapping and Stream classification.
- [x] Run go test ./internal/blehelper ./internal/ble ./internal/probe -count=1;
  expect all pass, including existing generic cleanup-fatal tests. Build native
  helper to validate darwin wrapper. Commit Task2 files.

### Task 3: Acceptance, documentation and delivery

**Files:** docs/PLAN.md, docs/STATUS.md, docs/NEXT_CONNECTION.md, README.md;
optional focused Source integration tests only if existing coverage has a gap.

- [x] Run go test ./... -count=1; go test -race ./... -count=1; make lint;
  make test-web; make build-dashboard; make build-obdprobe; make build-milanalyze.
  Expect all pass; rerun sandbox-denied required commands with approval.
- [x] Update documentation with3-check escalation, recognized native timeout
  recovery, limitations and separate parked ignition/sleep acceptance. Record
  tests and input immutability in STATUS; no hardware acceptance claims.
- [ ] Independent whole-branch review; repair material findings with RED/GREEN.
  Diff check, explicit-file commit, push feature branch; no automatic main merge.
