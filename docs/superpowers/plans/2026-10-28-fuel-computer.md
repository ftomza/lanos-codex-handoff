# M6 Fuel Computer Implementation Plan

> Execute inline using executing-plans and test-driven-development. No delegation.

**Goal:** Persistent, resettable and calibratable fuel accounting in simulator
and explicitly experimental live telemetry, displayed on the existing dashboard.

**Architecture:** internal/fuel owns validated estimators, bounded monotonic
accounting and atomic state. telemetry defines its transport view. server owns
one computer and updates it from one source reader plus a 250ms tick; HTTP only
reads views/mutates local trip state. Browser owns presentation and watchdog.

**Tech Stack:** Go 1.27 stdlib, existing BLE helper, HTML/CSS/ES modules/Node tests.
No module/dependency changes. Approved spec: 2026-10-28-fuel-computer-design.md.

## 1. Estimators and wire view

Files: internal/fuel/config.go, estimate.go, estimate_test.go;
internal/telemetry/fuel.go.

- [x] Add table tests before implementation: fresh synthetic signals and config
  defaults, auto priority direct/MAF/speed_density, expected SI numeric result,
  live unverified IAT gating, out-of-range/-40/NaN/stale/missing/zero RPM.
  Core assertion: Evaluate(DefaultConfig(), snapshot, "speed_density", now)
  gives 0.75 VE airflow converted with 14.7 AFR and 745 g/L, never a trim/DFCO guess.
- [x] `go test ./internal/fuel` must fail on absent APIs. Implement Config with
  Validate/Fingerprint, Estimator interface, three concrete strategies and
  Evaluate/Select helpers; return null + reason on every invalid dependency.
- [x] Define telemetry.Fuel view with independent flow/speed expiry, totals,
  coverage, model settings, revision/calibration and persistence status. Add At
  helper that expires only instantaneous values; historical totals remain.
- [x] Run estimator/domain tests; require exit 0.

## 2. Monotonic trip and mutations

Files: internal/fuel/computer.go, accounting.go, state.go, controls.go;
computer_test.go, controls_test.go.

- [x] Test 1s synthetic endpoints at constant 36 km/h / known L/h, with exact
  trapezoid expected liters and km. Add missing fuel but valid speed, idle with
  missing speed, generation changes, >3s dt, backwards/duplicate times, timeout,
  restart and method-change cases. Average uses jointly covered liters/km.
- [x] Observe missing computer APIs, then implement serialized Observe/Tick/View
  with bounded latest input and counters, no stored context/history. Expiry
  rejects intervals extending beyond either endpoint input's validity horizon.
- [x] Test Reset and calibration preview/apply/undo/default with expected revision,
  full-to-full confirmation, >=10 covered km, complete interval, factor [0.25,4].
  Inject a failing Save; mutation must preserve old committed trip and report error.
- [x] Implement versioned State and Store interface, raw/joint totals, incomplete
  reasons, durable-before-success mutations and preserved calibration on reset.
  Explicit startup reset permits source/config changes and restores factor 1.
- [x] `go test ./internal/fuel -count=1` must pass.

## 3. Atomic file state

Files: internal/fuel/store.go, store_test.go; .gitignore.

- [x] Test new/reopened private JSON, exclusive sidecar writer lock, strict
  version/numeric/schema/source/config validation, corrupt/truncated/extra JSON,
  failed rename/write and idempotent Close. Restart restores totals, not anchors.
- [x] Implement Open file store with 64KiB read bound, strict decoder, 0600 temp
  file + sync + rename, 0700 newly created directory, owner-created sidecar lock.
  Checkpoint every <=5s and on Close/mutations; never remove unknown locks.
- [x] Add /data/fuel/ ignore, wire Open(Config,source,path,reset) and tests.
- [x] Run domain/storage tests including -race; require exit 0.

## 4. Source signals and shared service

Files: internal/live/events.go, internal/probe/schedule.go,
internal/telemetry/snapshot.go, internal/sim/sample.go and relevant tests;
internal/server/server.go, fuel.go, fuel_test.go.

- [x] Add tests first for MAP/IAT/MAF/fuel_rate signal TTL/provenance and generation
  increment; supported-only schedule adds 10/5E, observed Lanos never queries them.
  Simulator emits deterministic synthetic MAP/IAT and unchanged legacy phases.
- [x] Implement those adapter extensions, retain all old probe/record tests.
- [x] Test server with real fuel computer/temp store, fake source, two clients,
  shared increasing totals and tick-based expiry when source stops responding.
- [x] Implement one cancellable read goroutine + buffered channel and 250ms server
  tick, Observe on updates, Tick on timer, fuel view in snapshot/SSE. Close joins
  source worker and persists computer; no per-browser computation/adapter opens.
- [x] Run probe/live/sim/server tests; require exit 0.

## 5. CLI and same-origin local controls

Files: cmd/dashboard/fuel.go, flags.go, main.go, fuel_test.go;
internal/server/fuel_http.go, fuel_http_test.go.

- [x] Tests first: defaults and model overrides, --fuel=off, IAT opt-in, state
  override/reset, invalid config/help/bind fail do not create state/open device;
  corrupt/locked state fails before Source.Start. Use temp files in command tests.
- [x] Wire flags for fuel auto/off, model auto/direct/maf/speed_density, VE/AFR/
  density/displacement/threshold, state and explicit reset; validate all settings.
- [x] Tests first for JSON POST reset/calibration preview/apply/undo/default:
  same-origin + JSON + <=4KiB strict body + expected_revision, rejection of
  cross-origin/unknown fields/stale edits, failed persistence and disabled module.
- [x] Implement handlers through computer local methods; never touch transport.
  Config/off commands retain old application lifecycle and source error evidence.
- [x] Run CLI/server tests; require exit 0.

## 6. Dashboard fuel panel

Files: web/fuel.mjs, fuel.test.mjs, app.mjs, index.html, style.css; Makefile.

- [x] Add Node tests before implementation for low-speed L/h vs L/100km,
  unavailable speed, independent flow/speed expiry, incomplete/model/unsaved
  notes, null/nonfinite values and retained historical totals on disconnect.
- [x] Implement pure fuelView(event,elapsedMs,transport) and compact panel for
  instantaneous/average/liters/distance, calibration/coverage note and controls.
  Explicit dialogs/forms preview and confirm reset/full-to-full factor changes;
  revisioned JSON mutations, loading/error feedback, no automatic reset.
- [x] Include new module/tests in make test-web; syntax check all changed modules.
  CSS keeps existing instruments and supports 1280x800/narrow screens.
- [x] Run browser tests; require exit 0.

## 7. Acceptance and delivery

- [x] Review every approved spec section against implementation/tests. Reproduce
  bugs with failing regressions before fixes; preserve owner files/private logs.
- [x] Run go test ./... -count=1, go test -race ./..., make lint, make test-web,
  make build-dashboard, make build-obdprobe, Linux cross-build, native helper vet,
  CLI help, simulator HTTP/SSE/mutations/restart and git diff --check. No device opens.
- [x] Update README, docs/FUEL_COMPUTER.md, PROJECT/PLAN/STATUS/NEXT_CONNECTION,
  approved spec and this checklist with exact verification and remaining hardware
  limits. No accuracy claim; IAT/full-to-full validation remain pending.
- [x] Commit intended files with required trailer and push feat/fuel-computer.
  Rebuild binaries for final revision. Keep test.py/test2.py and private state out.

## Execution result

Owner-approved scope implemented offline on 2026-10-28, no delegation/hardware.
Go/race/lint/build checks, 22 browser logic tests, real simulator HTTP/SSE/state/
reset/restart/shutdown smoke and DOM-stub interaction smoke passed. Additional
regressions fixed preview drift using a token, kept recorder scheduling unchanged,
embedded both browser modules and preserved UNKNOWN labels. Native visual/browser
and physical fuel accuracy remain pending and are explicitly documented.
