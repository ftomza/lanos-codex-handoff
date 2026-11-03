# M3.3.1–M3.3.4 MIL context capture

Scope: owner-supplied M3.3 in docs/PLAN.md and explicit request to implement its
first four stages. Same checkout, feat/mil-diagnostics from d95da3a. No physical
connection, UI work, automated root-cause correlation or proprietary commands.

M3.3.1 is documented in diagnostics/FIRST_TRIP_ANALYSIS.md. The exact private
history is retained unchanged. No saved MIL means no inferred onset timestamp.

M3.3.2 adds 0101 readiness, separate permanent Mode 0A and frame-00 Mode 02 reads.
Finite CLI flags explicitly opt in; old commands/defaults remain. Mode 02 first
reads 020000, then only advertised first-block PIDs relevant to engine context,
including frame DTC PID 02. Raw negative replies distinguish explicit service
unsupported from NO DATA/unavailable/malformed. ATH1 is finite diagnostic-only:
KWP header length/checksum/responder validation, raw retained; unsupported header
formats fail explicitly rather than silently removing bytes or guessing units.
Never allow Mode04, actuator/write/config/programming/VIN/arbitrary commands.

M3.3.3 uses one existing serialized Runner stream. Opt-in MIL recording requests
0101 at 5s (configurable 1s..60s), with per-signal freshness and UNKNOWN for loss.
The monitor recognizes initial ON (MIL_ALREADY_ON), confirmed adjacent OFF/ON
transitions, and observations after gaps without inventing transitions. Generation,
raw/received time and stale age are explicit; duplicate capture timestamps do not
create repeated events. A timestamped 60s ring holds at most one context sample
per second plus forced state boundaries, with a hard count limit. Old samples
are never relabelled fresh; gaps and late-start coverage remain explicit.

M3.3.4 attaches one capture manager to the live Source before Start. It receives
all source state changes independently of browser count, enriches the record
with a read-only view of the existing Fuel Computer (never advancing it), and
sends immutable rows to a bounded asynchronous writer. Snapshot capture covers
available 60s pre/30s post observations. Early shutdown/loss/overflow/retention is
marked incomplete. Multiple nearby events share a bounded active incident and
retain each transition; the deadline has a hard bound to avoid indefinite capture.

Automatic DTC work is low-priority, one serialized request per core round,
interleaved with ordinary support queries; MIL due checks take priority in the
support slot. Unsupported modes are not retried every tick. Frame-zero bitmap
controls subsequent freeze-frame PID reads. Existing timeouts, prompt recovery,
core sampling and fuel formulas/state remain; no second Vgate connection.

Flags: dashboard --mil-record-dir enables recording and shared scheduling;
--mil-interval defaults to 5s. Simulator can exercise monitor capture with fixtures;
real readings stay source-labelled. New managed files are 0600, directories0700,
exclusive writer lock; bounded managed retention and queue failures visible in
API/log/close result while source and fuel remain active. No owner files deleted.
History API/UI is deferred to M3.3.6; JSONL is inspectable offline with stdlib tools.

Verification: real saved raw fixtures only for actual observations; explicitly
synthetic NRC/readiness/header/freeze/MIL scenarios. Parser, guardrails, schedule,
transition/gaps/dedup, pre/post, early close, output failure and two-browser tests.
Full Go/race/vet/lint/web/build and synthetic end-to-end capture. STATUS after each
stage; physical M3.3.7 remains separately authorized and unexecuted.
