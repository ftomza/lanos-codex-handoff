# M6.1 Fuel Analytics Implementation Plan

> **For agentic workers:** Use executing-plans and test-driven-development inline.
> Work in the existing feat/fuel-analytics checkout as requested by the owner.

**Goal:** Record explainable bounded fuel history and analyze/replay it offline.

**Architecture:** Computer emits immutable coherent diagnostics to a nonblocking
history writer. A streaming reader/analyzer serves CLI and history HTTP endpoints;
the separate browser page renders only saved series. Existing fuel state v1 and
ECU schedule remain unchanged.

**Tech Stack:** Go 1.27 standard library, existing native BLE, plain JS/Canvas.
No new dependencies. Approved spec: ../specs/2026-10-28-fuel-analytics-design.md.

## 1. Diagnostic domain and coherent computer observations

Files: internal/telemetry/fuel_diagnostics.go, internal/fuel/diagnostics.go,
iat.go, estimate.go, computer.go, controls.go; internal/live/events.go.

- [x] Add tests for speed-density intermediate units, quality, IAT range/jump/
  constant/cold-start flags, expiry and unique-measurement timestamps; test
  observer samples/reset/calibration and immutable coherent state.
- [x] Observe missing diagnostic API failures with `go test ./internal/fuel`.
- [x] Add nullable intermediates to the existing Result calculation; additive
  Fuel quality fields; extract locked view for atomic Diagnostic snapshots.
- [x] Add bounded IAT state, explicit cold annotation, timed observer and control
  events. No state-v1/fingerprint/formula changes or observer I/O under the lock.
- [x] Expose already-polled throttle/trims/O2 with independent TTLs, including
  O2 extra trim only when the existing decoder supplies it. Verify no new writes.
- [x] Run fuel/live tests and old state/API compatibility regressions.

## 2. Bounded JSONL storage

Files: internal/fuelhistory/{types,manifest,writer,reader}.go and tests.

- [x] Add failing real-file tests for private mode, source/lock exclusion,
  rotation and retention, restart, malformed manifest/paths, output errors,
  partial tail, bounded queue, sequence/version gaps and clean close.
- [x] Implement schema-v1 envelopes, atomic bounded manifest, owned numbered
  parts, separate exclusive lock and 256-row nonblocking queue.
- [x] Rotate before size bound; delete only manifest-owned closed files. Fsync
  at <=5s and close. Runtime failures stop recording and remain visible.
- [x] Implement context-aware bounded streaming reader; retain complete rows,
  report corruption/truncation and never bridge a damaged boundary.
- [x] Run `go test ./internal/fuelhistory -count=1`.

## 3. Streaming analysis and CLI

Files: internal/fuelhistory/{analyze,intervals,series}.go; cmd/fuelanalyze/.

- [x] Test observed durations, authoritative counter deltas, sampled integration
  comparison, idle/moving/unknown, weighted averages, distributions/mixed bins,
  minute/generation/reset/model/factor segments, gaps, IAT anomalies and restart.
- [x] Implement shared streaming Analyze(ctx, dir, options) and session listing.
  Use counters only across validated comparable endpoints; never count a reset
  or factor jump as consumption. Keep observed and unknown periods separate.
- [x] Bound chart series to 4000 points each with extrema/gap-preserving buckets;
  bound issue/event lists, segment count and all untrusted input lengths.
- [x] Add fuelanalyze --input/--json/--session with cancellable text/JSON reports.
  Parsing/help never opens hardware or persistent fuel state.
- [x] Run analyzer/CLI tests; compare synthetic known-unit replay with M6 totals.

## 4. Dashboard/CLI integration

Files: cmd/dashboard/{flags,fuel,history,main}.go, internal/server/history.go,
server.go, http.go; corresponding tests; Makefile; .gitignore.

- [x] Add failing flag/lifecycle/API tests: opt-in, bounds/path matching,
  fuel-off history view, startup failure before Source.Start, shared recorder,
  safe session/range requests, two readers and actual close errors.
- [x] Register/validate record/history flags; open history before source Start,
  attach Computer observer and pass history to server. Close computer before
  recorder. History reads never reset or advance fuel counters.
- [x] Serve list/report endpoints from configured directory only, using bounded
  requests/context deadlines. Expose writer failure/status in live API.
- [x] Keep previous sim/live startup, cancellation and BLE standby behavior.
- [x] Run dashboard/server tests and both command help/build checks.

## 5. History screen

Files: web/{analytics.html,analytics.mjs,analytics-model.mjs,analytics.test.mjs},
style.css, index.html, app.mjs/fuel.mjs as needed, embed/Makefile tests.

- [x] Write browser-model tests for history source labels, gap paths, missing
  data, extrema/downsampling labels, model/quality and reset/factor markers.
- [x] Add separate saved-session screen in existing style, session/range controls,
  report tables and six Canvas series. Draw gaps and event markers explicitly;
  never substitute the live snapshot or invent missing points.
- [x] Show input quality and recorder failures in the existing fuel panel.
- [x] Run `make test-web`; test embedded routes and offline-view mode.

## 6. Verification and delivery

- [x] Run full Go tests, race tests, make lint, web tests, native dashboard/probe
  and analyzer builds, syntax/diff checks. Review approved spec against final code.
- [x] Run built simulator record -> close -> restart -> CLI -> HTTP/UI replay
  with isolated private state. Verify two SSE clients and graceful SIGINT.
- [x] Update STATUS, FUEL_COMPUTER, NEXT_CONNECTION, README and approved spec/plan
  with actual commands, outcomes, limitations and next physical recording steps.
- [x] Commit only intended files with required trailer; push feature branch.
  Leave owner test.py/test2.py, private data and main untouched.


Verification record: full Go/race/lint checks and 26 web tests passed; native
binaries built. Final simulator recording/restart/CLI/HTTP and isolated WebKit
at 1280/560 passed; see STATUS.md for commands and synthetic artifact paths.
No physical adapter, owner private data, dependency or ECU-schedule changes.
