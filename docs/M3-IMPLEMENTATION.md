# M3 — Vgate read-only probe implementation plan

Source: https://github.com/ftomza/lanos-codex-handoff/issues/1 (read 2026-10-24;
initial comments fetch returned none). The subsequent owner correction is
https://github.com/ftomza/lanos-codex-handoff/issues/1#issuecomment-6383158184:
device model Vgate iCar Pro BLE 4.0 Dual, confirmed Classic SPP on MacBook M3.
Use Serial/SPP now; defer BLE GATT until discovery, without UART/concurrency
assumptions. Model context and its provenance are separate from ATI identity.
Existing M0–M2 and the user's untracked `test.py` are preserved.
Branch: `feat/m3-vgate-probe`. No live device access during implementation.

## Boundaries and decisions

Use Go stdlib only; keep go.mod unchanged. Native macOS nonblocking serial I/O
and termios configuration belong in internal/serial. Other hosts can run offline
tests, but opening a real port is explicitly unsupported there. No stty subprocess
or external tool is required by tests.

internal/elm327 owns strict read-only command validation, serialized CR-framed
commands, bounded responses, prompt completion, normalization and decoders.
A command timeout poisons the session: another command cannot be written until
a delayed prompt is consumed. Recovery logs the drained bytes, then initialization
and discovery restart. If synchronization cannot be recovered, stop. Reopening
after a transport loss also requires synchronization before sending more commands;
do not silently discard input or claim Bluetooth pairing can be repaired.

internal/probe composes initialization, capability discovery, safe sampling,
bounded reconnect/backoff and a timestamped JSON report. Partial results remain
explicitly partial; NO DATA is temporary unavailability, not unsupported or zero.
Every connection attempt rebuilds capabilities and readings to avoid stale values.
Only bitmap-declared PIDs are sampled. Continuation queries require bit 32.
03 is an independent read-only service; 0101 may report MIL when advertised.
Do not request VIN. Redact recognizable unsolicited VIN text/Mode 09 VIN lines
in exported raw data by default; offer an explicit redaction flag.

cmd/obdprobe provides --list-ports, --port, --baud, --timeout,
--initial-timeout (30s), --scan-supported, --json, --output and bounded retry
configuration. --port is mandatory for hardware access. Output files use 0600
and exclusive creation to preserve previous diagnostic logs.

## Ordered tasks (tests first)

- [x] Save the exact escaped Issue #1 session as a JSON fixture with provenance.
  Add failing tests for echo/SEARCHING, A5, BE3EB811, zero RPM/speed, 78°C and
  empty stored DTCs; implement parsing/decoding and malformed/error cases.
- [x] Add command framing tests for fragmented prompts, partial writes, timeout,
  late responses, overflow and rejected writes. Implement the serialized session
  and read-only recovery. Run `go test ./internal/elm327`.
- [x] Add offline port enumeration, configuration and cancellation tests; implement
  macOS raw 8N1 9600/38400 transport with context-aware nonblocking operations.
- [x] Add fixture-backed probe tests for capability gating, unknown continuation,
  command deadlines, partial results, timeout recovery and disconnect/reopen.
  Implement bounded retries with no blind buffer flush or source substitution.
- [x] Add CLI tests for no implicit device access, output/redaction and validation;
  implement the CLI. Run help and offline fixture examples only.
- [x] Document confirmed observations in capabilities.md and next-car-session
  instructions; update README and STATUS.md. Propose M4 separately.
- [x] Run gofmt, go test ./..., go test -race ./..., go vet ./..., go build ./...,
  make lint, existing JS tests, Linux cross-build and git diff --check. Record
  exact results and physical-test limitations in STATUS.md.
