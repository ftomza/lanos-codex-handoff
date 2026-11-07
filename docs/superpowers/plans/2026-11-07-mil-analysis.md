# M3.3.5 MIL Analysis Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Analyze existing private MIL archives offline, preserving provenance,
uncertainty, gaps and independent diagnostic results.

**Architecture:** A read-only streaming reader in milstore feeds milanalysis.
The analyzer groups bounded incident context, then applies deduplicated,
freshness-aware heuristics. A separate CLI renders Russian text or versioned JSON.

**Tech Stack:** Existing Go 1.27 module, standard library, testing, existing MIL
and telemetry types. No transport changes/dependencies. Approved specification:
`docs/superpowers/specs/2026-11-07-mil-analysis-design.md`.

## Task 1: bounded read-only archive reader

Files: create `internal/milstore/reader.go`, `reader_part.go`, `reader_test.go`.
Reuse internal manifest validation; avoid changing writer behavior.

- [x] Write failing tests using a closed archive produced by the existing
  recorder. Assert streamed envelopes preserve session/sequence/file/line,
  `pre.at` may precede an event, no lock/mtime/content change, selected session
  is validated. Add synthetic corrupt-tail, sequence, version, footer, aborted,
  cancellation, symlink/traversal, rotation, byte-limit and sink-error cases.
- [x] Run `go test ./internal/milstore -run TestRead -count=1`; expect new API
  missing, then failing behavior if stubs are introduced.
- [x] Implement API:

```go
type ReadOptions struct { Session string }
type ReadEntry struct {
    Version int `json:"version"`
    Session string `json:"session_id"`
    Source string `json:"source"`
    Sequence uint64 `json:"sequence"`
    ElapsedNS int64 `json:"elapsed_ns"`
    Entry
    File string `json:"-"`
    Line int `json:"-"`
    Boundary bool `json:"-"`
}
type ReadIssue struct {
    Code string `json:"code"`
    File string `json:"file,omitempty"`
    Line int `json:"line,omitempty"`
    Detail string `json:"detail"`
}
func Read(context.Context, string, ReadOptions, func(ReadEntry) error) ([]ReadIssue, error)
```

  Stream at most 1MiB/row, 64MiB/part, respect context and close errors.
  Issues break continuity and invalid rows never reach the sink. Retention and
  nonclosed files are issues. Unknown requested session is an error.
- [x] Re-run package tests; review spec compliance and code quality before
  delivery. Commit only explicit task files, with required attribution trailer.

## Task 2: report assembly and evidence rules

Files: create `internal/milanalysis/types.go`, `analyze.go`, `incidents.go`,
`rules.go`, `temporal.go`, `quality.go`, their focused `_test.go` files and
`testdata/README.md` documenting fixture provenance.

- [x] Write failing synthetic tests that build archives with the recorder and
  assert OFF/ON/UNKNOWN histories, no fabricated onset, separate DTC statuses,
  incomplete windows, no inferred attachment of late diagnostics and bounded
  deterministic output. Exercise selection/context cancellation and malformed
  typed payloads, not only JSON syntax.
- [x] Run `go test ./internal/milanalysis -count=1`; expect missing feature.
- [x] Implement report assembly through reader callback. Maintain per-session
  and per-incident scope, use existing `mil.Event`, `mil.Frame`, `mil.Window`
  and `telemetry.ECUReading` without changing their JSON contracts.
- [x] Write rule tests before implementation, covering exact thresholds in
  the approved spec, stale/future/duplicate/wrong-unit evidence, generations,
  gaps, sparse O2, too-short constant-IAT context and matched-input skew.
- [x] Implement one ruleset at a time. A rule's evidence contains bounded
  samples with file/line/sequence/capture-time/value/unit and observation count.
  Never carry temporal state across scope/generation/integrity boundaries.
- [x] Run `go test ./internal/milanalysis ./internal/milstore -count=1` and
  focused race checks. Independent spec and quality review; fix findings.

## Task 3: CLI and delivery

Files: create `cmd/milanalyze/main.go`, `text.go`, `main_test.go`; modify
`Makefile`, `README.md`, `docs/MIL_DIAGNOSTICS.md`, `docs/PLAN.md`, `docs/STATUS.md`.

- [x] Test `--help`, missing/invalid arguments, session-scoped incident filter,
  input error, output writer error, text timezone and JSON schema/determinism.
  Use synthetic temporary archives so `go test ./...` needs no private files.
- [x] Implement `run(ctx, args, out)` using `flag`, context timeout30s,
  signal.NotifyContext in main, and `json.Encoder` or Russian text renderer.
  Keep CLI imports free from probe/BLE/serial/live source packages.
- [x] Add build target:

```make
.PHONY: build-milanalyze
build-milanalyze:
	go build -o bin/milanalyze ./cmd/milanalyze
```

- [x] Run `go test ./... -count=1`, `go test -race ./... -count=1`, `make lint`,
  `make test-web`, `make build-milanalyze`. Run text/JSON for today's session,
  verify P0420/unsupported0A/incomplete post/sparse O2, and unchanged hashes of
  private originals. Do not open hardware.
- [x] Update docs with actual commands/results/limits, mark only M3.3.5 complete.
  Final independent review and `git diff --check`; stage explicit files and
  commit with `Co-Authored-By: O3 Agent Codex <noreply@ozon.ru>` exactly once.
  Keep owner test.py/test2.py untouched and M3.3.6 deferred.
