# M3.3.6 Dashboard Diagnostics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show live MIL, historical DTC/event evidence and bounded archive exports
in the existing dashboard without changing vehicle polling or fuel accounting.

**Architecture:** Extend read-only MIL catalog/history HTTP services and CLI
configuration, then add an embedded diagnostics page using the existing SSE Feed
and a pure presentation model. Keep live source state separate from saved reports.

**Tech Stack:** Existing Go1.27 standard library, embedded HTML/CSS/ES modules,
Node built-in tests. No Go/dependency changes.

**Spec:** `docs/superpowers/specs/2026-11-07-mil-dashboard-design.md` (approved).

## Global Constraints

- One existing ELM source/session; no new ECU commands, polling, clear/write.
- Preserve current fuel model/state, archive formats and source defaults.
- Current MIL must become UNKNOWN for stale/future/unavailable/transport loss.
- Archive LIVE and current SIM are distinct; no silent data substitution.
- Query <=2048 bytes, no filesystem paths; exact known keys once each.
- Report/export:8sec context, one concurrent request per Server, no queue,
  8MiB response limit. Browser timeout10sec; archive scans never run per SSE.
- Recoverable reader issues:200/incomplete; fatal manifest/open/rotation:409;
  invalid query400, disabled503, occupied429, timeout504. Oversize output413.
- Use textContent/DOM for saved content; existing CSP/no-store/nosniff remain.
- 1280x800 and narrow layout; physical Android/hardware claims require evidence.
- Do not inspect/modify owner test.py/test2.py, private recordings, .env*, .idea,
  sensitive home config/credentials. No private raw trip files in commits.

## Review Focus

- A failed report fetch after selection changes clears previous download links.
- Retention/active/corrupt rows cannot turn surviving evidence into empty success.
- DTC lists retain timestamp/status; MIL freshness does not make old DTCs current.
- Concurrent browser report requests cannot serialize behind a telemetry lock.
- Offline SIM viewing a LIVE archive cannot enable hardware or MIL polling.

### Task 1: read-only MIL archive HTTP and launch configuration

**Files:** Create `internal/milstore/catalog.go`, `catalog_test.go`,
`internal/server/mil_history.go`, `mil_export.go`, `mil_history_test.go`;
modify `internal/server/server.go`, `http.go`, `cmd/dashboard/flags.go`,
`mil.go`, `mil_test.go`, `main.go`.

**Interfaces:**
- Consume `milstore.Read(ctx, dir, ReadOptions, sink)`, `milanalysis.Analyze(ctx,
  dir, Options)`, immutable existing server snapshots and Config.
- Produce `milstore.List(ctx context.Context, dir string) (Catalog,error)`.
  Catalog JSON: version, source, retention_truncated, sessions[]. Session JSON:
  id, started_at, parts, closed, aborted. closed describes all retained files,
  never diagnostic health. List validates only manifest/files, not JSONL scans.
- Add `server.Config.MILHistoryDir string`, options `milHistoryDir string`,
  flag `--mil-history-dir`. Record-dir implies history-dir; both must match if
  explicit, and neither may equal fuel record/history paths.
- GET `/api/mil/history/sessions` returns Catalog; no query allowed.
- GET `/api/mil/history/report?session=<id>[&incident=<id>]` returns Report v1.
  Session mandatory; validate existing milanalysis.Options and reject duplicates,
  unknown keys/malformed query/oversized query. No directory in URL.
- GET `/api/mil/history/incident?session=<id>&incident=<id>` returns export v1:
  scope="incident_export", source, session_id, incident, filtered=true,
  rows[], issues[], incomplete. Rows embed the original ReadEntry JSON envelope
  plus file/line provenance. Include matching incident rows and session boundaries,
  not independent late responses. Unknown object ->409. Active/retained/corrupt
  context ->200/incomplete. Export is a validated filtered view, not managed input.

- [ ] Write synthetic TDD tests for catalog/closed/rotation/retention/cancellation
  and no content/mtime/lock changes. Add handler tests for valid and invalid
  selections, disabled/fatal/recoverable history, active/failed/partial export,
  unassociated late data, response limit and content headers. Expected assertions:

```go
if rr.Code != http.StatusOK || !export.Filtered || export.Scope != "incident_export" { t.Fatal(rr.Body.String()) }
if !report.Incomplete || len(report.Issues)==0 { t.Fatal("lost recoverable damage") }
if rr.Code != http.StatusBadRequest { t.Fatal("accepted unknown or repeated query") }
```

- [ ] Run new focused tests to observe failure before implementation.
- [ ] Implement List using existing strict readManifest/readDirectory helpers.
  New endpoints run outside s.mu; initialize a separate one-slot channel per
  Server and reject rather than queue. Bounded buffered JSON encoding must
  fail before writing a successful response. Error messages must not leak paths.
- [ ] Implement flag normalization/read-only wiring. History-only never calls
  milstore.Open/NewCapture or changes probe MILInterval. Missing configured
  history is an HTTP error, not a reason to stop core telemetry at startup.
- [ ] Add concurrency tests by holding the one-slot gate and proving both
  report/export return429 while `/api/snapshot` and catalog still succeed.
  Add request cancellation/deadline checks and SIM/live catalog separation.
- [ ] Run `go test ./internal/milstore ./internal/server ./cmd/dashboard -count=1`
  and focused race tests; format and provide a report for review. Root handles
  explicit-file staging/commit due shared workspace git permissions.

### Task 2: diagnostics page, freshness model and documentation

**Files:** Create `web/diagnostics.html`, `diagnostics.mjs`,
`diagnostics-model.mjs`, `diagnostics-history.mjs`, `diagnostics.test.mjs`;
modify `web/style.css`, `index.html`, `analytics.html`, `embed.go`, Makefile,
README, docs/MIL_DIAGNOSTICS.md, docs/NEXT_CONNECTION.md. PLAN/STATUS final
completion is owned by the root after fresh global verification.

**Interfaces:**
- Consume Task1 HTTP contracts and existing `/events` + Feed, Event source,
  health/server_time/stale_after_ms, mil/ecu_readings/mil_recording.
- Produce `/diagnostics.html`; named export
  `milView(event,elapsedMs,transportStatus)` in diagnostics-model.mjs for pure
  tests. Other helper names are builder choices. History controller must support
  injected fetch/onState so selection/cancel/error races can be tested in Node.
- Fresh MIL uses independent captured/received times/TTL and current source
  freshness. Count integer0..127 only with valid fresh MIL. Last-success label
  is explicitly scoped to this opening of the page, never inferred from failure.
- DTCs are timestamped last-known responses, never called simultaneous/current.
  Require consistent available status and decoded codes array before empty success;
  distinguish no query/unsupported/unavailable/invalid. Validate future timestamps.
- Summary for unexplained fresh ON is exactly:
  «Check Engine включён. Причина не определена стандартными диагностическими режимами».

- [ ] Write Node tests first: ON/OFF/UNKNOWN, stale/future/silent SSE, source and
  transport expiry, SIM and source mismatch; invalid count/DTC shape, recorded
  failure and independent last-known timestamps. Example boundaries:

```js
assert.equal(milView(event, 15000, 'open').state, 'UNKNOWN');
assert.equal(milView(event, 0, 'retrying').state, 'UNKNOWN');
assert.equal(milView(event, 0, 'open').state, 'ON');
```

- [ ] Add controller tests for stale fetch completing after a new selection,
  errors disabling old links/report, cancellation, empty/disabled/corrupt history,
  unsupported schema, response limit and source labels. Observe tests failing.
- [ ] Implement page/model/history with safe DOM/textContent and current theme.
  One lightweight catalog and latest-session report load on opening; explicit
  selection/refresh thereafter. No archive work in SSE/watchdog callbacks.
  Use monotonic performance time for watchdog; bounded Feed retries and retry UI.
- [ ] Render events, DTC/freeze replies, flags/limits/issues, all finding fields
  with evidence provenance and insufficient-data checks. Provide JSON report link
  and exact session/incident export links; no path-based/raw-file endpoint.
  Selection immediately clears old content and export affordances.
- [ ] Add navigation, production embed entries and new Node test to make test-web.
  Add documentation for live existing flags and offline viewing:

```sh
make build-dashboard
./bin/dashboard --source=sim --fuel=off --mil-history-dir=data/mil-events/live
```

  Confirm actual existing fuel-off flag syntax before publishing the command.
  Explain separate SIM/LIVE labels, partial active history, read-only exports,
  manual refresh and unchanged live/fuel launch command.
- [ ] Run `make test-web`, `go test ./internal/server ./cmd/dashboard -count=1`
  and provide report for task review; root handles explicit staging/commit.

## Final verification and delivery (controller)

- [ ] Run go test ./... -count=1, go test -race ./... -count=1, make lint,
  make test-web, make build-dashboard, make build-milanalyze.
- [ ] Run a built SIM server with synthetic/read-only saved MIL history. Verify
  navigation/assets/API, P0420 report and filtered snapshot download, disabled/
  empty/error states, output limits and unchanged input hashes/mtimes.
- [ ] Use an available local browser to inspect 1280x800 and narrow viewport;
  if unavailable, document that limit without claiming visual/Android acceptance.
- [ ] Whole-branch independent review; address findings through task implementer.
  Update PLAN/STATUS with actual evidence, scope limits and M3.3.7 next.
- [ ] Commit and push feat/mil-dashboard with required trailers; never stage owner
  scripts/private reports or merge main automatically.
