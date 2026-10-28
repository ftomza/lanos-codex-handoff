# Live Dashboard Implementation Plan

> **For agentic workers:** Execute inline with superpowers:executing-plans. No delegation is authorized. Use test-driven-development for each behavior below.

**Goal:** Feed the four existing dashboard instruments from native read-only BLE/serial with independent freshness.

**Architecture:** Expose an unbounded-context probe Stream that shares recorder
initialization/polling/recovery. A native live.Source converts typed events to
immutable bounded latest snapshots. Keep the existing HTTP/SSE server, adding
per-signal metadata to the compatible flat API and independent browser expiry.

**Tech Stack:** Go 1.27 standard library, existing Bluetooth dependency/helper, existing HTML/CSS/ES modules and Node tests.

### Task 1: Shared continuous stream and error evidence

Files: internal/probe/{record,record_events,record_test}.go.

- [x] Add tests for `Runner.Stream(ctx, sink)` continuing beyond 24h under fake
  monotonic time, cancellation, same allowlist/recovery and emitted cleanup error.
  Test contract: advance fixture clock by 25h after capabilities, accept another
  010C response, cancel, require `reason=canceled` and all old recorder tests pass.
- [x] Run `go test ./internal/probe` and observe missing Stream/Error APIs.
- [x] Share per-run initialization with Record, use duration=0 only in Stream,
  preserve duration validation for Record and total retry semantics. Add an
  optional error field to RecordingSummary and populate it before final emission.
- [x] Run `go test ./internal/probe ./cmd/obdprobe -count=1`, require exit 0.

### Task 2: Per-signal domain and native Source

Files: internal/telemetry/{snapshot,signals,signals_test}.go;
internal/live/{source,events,source_test}.go.

- [x] Add tests with `Snapshot{Signals: map[string]Signal{...}}`: new RPM does
  not refresh old coolant, expired signal serializes null, NO DATA has no value,
  At/Clone never mutate old maps/pointers, global silence/disconnect hides all.
- [x] Run `go test ./internal/telemetry` and observe missing Signal/Clone APIs.
- [x] Add optional signal metadata and immutable copy/expiry helpers; keep old
  simulator snapshots without Signals compatible. TTL 3s core, 35s coolant,
  130s ATRV; values retain independent captured/received timestamps and provenance.
- [x] Test `live.New(cfg, opener)` plus a fake stream/transport: Start/Read/Close,
  cancellation, immutable latest-only behavior, initial discovery, unsupported
  fields, NO DATA, gap reset, terminal exhaustion staying readable/disconnected.
- [x] Run `go test ./internal/live` and observe missing package/API, implement
  one worker and latest snapshot with notification channels, no context fields
  or growing queue. Reuse probe.Stream; return actual cleanup errors from Close.
- [x] Run domain/live/probe tests including -race, require exit 0.

### Task 3: Dashboard CLI and server integration

Files: cmd/dashboard/{main,flags,source,live_test}.go, Makefile;
internal/server/live_test.go.

- [x] Add validated source/transport flag tests and dependency-injected live
  startup. Assert help/bind/config failure do not open hardware and Source starts
  only after successful listener creation. Invalid live choices return an error.
- [x] Add HTTP/SSE integration with fake native Source and two clients: one poll
  worker, correct LIVE values, per-signal expiry and HTTP still available when
  the Source reports terminal disconnected state.
- [x] Run `go test ./cmd/dashboard ./internal/server`, observe new flags rejected.
- [x] Implement explicit sim/elm327 selection, BLE default for elm327, shared
  transport construction and listener-first startup. Keep run/serve compatible
  with simulator tests. Add build-dashboard producing dashboard plus BLE helper.
- [x] Run Go CLI/server/live tests, require exit 0.

### Task 4: Honest live browser presentation

Files: web/{model,model.test,app}.mjs, web/index.html.

- [x] Add Node tests: LIVE source descriptions, neutral initial HTML, no simulator
  outage text for live errors, stale coolant while RPM is fresh, independent
  browser elapsed-time expiry, missing/non-finite values, no stale warnings/arcs.
  Contract: `viewState(event, elapsedMs, 'open')` returns per-field display,
  numeric values for arcs and source-aware labels; a pure presentation helper
  owns descriptions so text behavior is tested without a browser dependency.
- [x] Run `make test-web`, observe expected failures before implementation.
- [x] Update model/app to use only fresh numeric values, dynamic source notice/
  notes and existing SSE retry controls. Keep layout/CSS and simulator behavior.
- [x] Run Node tests and syntax checks, require exit 0.

### Task 5: Shutdown evidence and final acceptance

Files as indicated by reproduced tests in internal/ble and/or blehelper;
README.md, docs/{PROJECT,PLAN,STATUS,NEXT_CONNECTION}.md, this plan/spec.

- [x] Exercise actual fake helper process cancellation/close; retain unexpected
  helper/close errors. If a new defect is reproduced, fix it only after a failing
  regression. Do not claim to know the owner log's absent cleanup error.
- [x] Run go test ./... -count=1, go test -race ./..., make lint, make test-web,
  make build-dashboard, make build-obdprobe, Linux cross-build, native helper
  vet, help/list-only smoke checks and git diff --check. No BLE device opens.
- [x] Document exact live launch, source identity, independent TTLs, finite retry
  policy, adapter voltage meaning and pending physical acceptance. Update STATUS
  with actual results. Self-review against every approved design section.
- [x] Commit intended files with required trailer and push feat/live-dashboard;
  exclude user test.py/test2.py and ignored recordings. Physical M4 acceptance
  stays pending; M6 remains separate.

## Execution result (2026-10-28)

Implementation and offline checks completed; no physical hardware access.
17 browser tests and all Go/race/build/lint checks pass. Source/HTTP integration
uses saved physical response bytes plus labeled synthetic inputs. Helper close
and cancellation tests retain real errors; the owner's old failure remains
unexplained. goimports is unavailable; gofmt/import review used. New physical
M4 dashboard acceptance is pending; follow NEXT_CONNECTION.md. M6 is separate.
