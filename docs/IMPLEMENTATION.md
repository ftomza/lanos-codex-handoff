# M0–M2 implementation notes

The requested scope is M0–M2 of PLAN.md, executed sequentially in this workspace.
No serial devices, adapter commands, ECU connections, or third-party dependencies.

## Design

- Domain: nullable measurements, capture/receipt timestamps, explicit source and
  health, and a pure freshness calculation. Missing values serialize as null.
- Source boundary: Start(context.Context), Read(context.Context), Close().
  Read blocks until a snapshot or cancellation; Close releases pending reads.
- Simulator: deterministic 60-second driving cycle, a brief voltage-unavailable
  interval, and a six-second simulated disconnect. Sampling time drives the cycle.
- Server: one source reader, synchronized latest snapshot, GET /api/snapshot,
  GET /events SSE, embedded static assets. Slow consumers receive latest state,
  with no growing per-client queues. Freshness is recomputed while streaming.
- UI: offline assets, large speed/RPM, coolant/voltage, permanent SIMULATOR label,
  source and transport health, receipt/capture times, client-side expiry watchdog.
  SSE reconnect uses capped exponential backoff with a finite attempt budget and
  a manual retry button. A feed reconnect immediately receives the latest state.
- Runtime: loopback by default, explicit LAN binding, cancellation and graceful
  HTTP shutdown. Only --source=sim is accepted in this scope.

## Milestone checklist

- [x] M0: initialize module, commands, package boundaries, Makefile and README;
  run go test ./..., go vet ./..., go build ./...; update STATUS.md.
- [x] M1: first add failing domain/simulator tests; implement freshness, missing
  values, deterministic phases and lifecycle; test cancellation and race safety;
  update STATUS.md before starting M2.
- [x] M2: first add failing HTTP/SSE and browser state tests; implement shared
  source runner, embedded UI, reconnection and shutdown; run formatting, all Go
  tests, race detector, vet, build, JavaScript tests and HTTP/browser smoke checks;
  update STATUS.md with exact outcomes and any verification limits.

## Verification boundaries

ELM framing/parsing, PID decoding and serial reconnect tests belong to M3/M4.
M0 only creates these package boundaries and an explicitly unavailable probe CLI.
Hardware and physical Android checks remain pending. The absence of
.codex/START_HERE.md is recorded in STATUS.md; PROJECT.md and PLAN.md supply the
scope and acceptance requirements.
