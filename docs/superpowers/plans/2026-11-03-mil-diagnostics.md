# M3.3 MIL Diagnostics Implementation Plan

Use subagent-driven-development for the isolated read-only protocol task and
review; execute remaining integration sequentially in the current checkout.
Owner authorized M3.3.1–M3.3.4 in docs/PLAN.md. No hardware access.

- [x] M3.3.1: stream-read exact private part; deduplicate signal capture times,
  inspect connectivity/generations/TTL/ranges, save FIRST_TRIP_ANALYSIS and STATUS.
- [x] M3.3.2: TDD elm327 readiness/permanent/frame00/ATH1 validation and narrow
  allowlist; finite probe opt-in and separate report fields; preserve defaults.
  Test known empty responses, NRC11/12, NO DATA, malformed/wrong headers/payloads,
  frame selection and unsupported outcomes. Run Go/race/lint/build, update STATUS.
- [x] M3.3.3: TDD domain MIL state/event/ring and Source snapshot propagation.
  UNKNOWN on loss/expiry; initialON, confirmed OFF/ON, gap resume, capture dedup.
  Configure 0101 due5s and serialized low-priority DTC job; test single transport,
  priority/fairness, bounds, no writes and recovery. Update STATUS.
- [x] M3.3.4: TDD managed private bounded JSONL incidents and capture lifecycle;
  attach observer before Start, include existing Fuel.View without integration,
  telemetry/source ages/raw diagnostic replies and poll latency/error context.
  Validate flags early, queue/retention/lock/failure/close, pre60/post30 and gaps.
  Run synthetic complete/aborted capture, two-browser and persistence checks.
- [x] Full go test ./... -count=1, go test -race ./... -count=1, make lint,
  make test-web, make build-dashboard build-obdprobe build-fuelanalyze build.
  Update README/NEXT_CONNECTION/PLAN/STATUS, commit intended files and push branch.
  Leave private records and owner scripts untouched; no M3.3.5+ implementation.
