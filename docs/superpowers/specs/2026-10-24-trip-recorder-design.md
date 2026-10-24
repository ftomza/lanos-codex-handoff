# Trip recorder for fuel-computer research

Status: proposed; awaiting owner review before implementation.

## Purpose and scope

Prepare a native Go read-only trip grabber that the owner starts on the current
MacBook before driving. Reuse obdprobe and its native BLE helper, with serial as
the existing alternative. This is a focused M3 utility follow-up to collect data
for M6. It does not complete dashboard/live Source M4 or implement fuel estimates.
No new dependency, Go upgrade, Python bridge or Android application is needed.
No hardware session is authorized by implementation of this design.

Recorded data can characterize engine operation and validate a future estimator.
It cannot uniquely determine fuel consumption or a volumetric-efficiency map
without independent fuel measurements. Full-to-full refueling data and distance
checks remain necessary for calibration. No claimed fuel accuracy from logs alone.

## Options considered

1. Recommended: add explicit recording mode to obdprobe. Reuses tested transport,
   allowlist, capability discovery, framing, decoding and recovery in one CLI.
2. Separate obdrecord command: clearer executable name, but duplicates CLI/setup
   unless that code is first extracted; unnecessary for this first recorder.
3. Build M4 dashboard/live Source first: useful later, but adds UI/server and
   freshness integration before the owner can collect research data.

## Proposed CLI

This interface is a proposal, not an available command yet:

```sh
make build-obdprobe
./bin/obdprobe --transport=ble --ble-name=IOS-Vlink --record --duration=4h
```

`--record` selects continuous trip recording instead of a finite diagnostic report.
`--duration` defaults to 4h in this mode; accept positive values up to 24h.
Reject an explicitly supplied duration without --record. Existing timeouts,
transport selection and --retries retain their meaning. Default retries are two
additional recovery attempts for the entire recording; do not reset this budget
after successful samples. Ctrl+C/SIGTERM and reaching duration finish cleanly;
exhausted retries or file/transport failures return a nonzero exit status.

Default output is an exclusive-create 0600 JSONL file under ignored diagnostics/,
with a timestamp and random suffix. Create the directory if absent; never
overwrite an existing file. --output can specify another new filename.
Reject --record together with --json, --list-ports or --scan-supported: recording
has its own streaming schema and automatically performs capability discovery.
Without --record, all existing CLI/report behavior remains compatible.

Print the output path, recording state and low-frequency counters to the terminal,
including command errors and reconnects. On exit print duration, sample/error/gap
counts and termination reason. Do not print an endless raw-response stream or
silently present old samples as current. Maximum file size is 256 MiB; stop with
an explicit size-limit result rather than overwrite or discard old data.

## Collection schedule

One ELM command at a time; no pipelining, arbitrary ECU requests or input flushes.
Reuse BLE initialization without ATZ and serial initialization unchanged.
Initially perform the existing capability-gated full scan and preserve all its
responses, including raw-only PID 03/13/21/22. Discover continuation bitmaps only
where indicated. Record adapter identity, protocol and source/profile metadata.

Poll only PIDs advertised in the current connection's bitmap:

- Core round: RPM 0C, MAP 0B, speed 0D. Target one round per second, measured from
  its start, without catch-up bursts when the ECU is slower.
- After each core round, read at most one due supporting item. Select the oldest
  due time, with ascending PID as a tie-breaker and ATRV last. Per-item target
  intervals: throttle 11 and fuel-system status 03 at 3s; STFT 06 and IAT 0F at
  5s; coolant 05, O2 14/15, load 04 and timing 0E at 10s; LTFT 07 at 30s;
  MIL/readiness 01 and ATRV at 60s. After a supporting read, its next due time
  is completion plus its interval, so overdue work does not cause bursts. Seed
  due times from initial scan results. This is an initial conservative schedule;
  actual per-PID intervals must be reported for analysis.
- Read stored DTCs in the initial scan of each segment; no repeated DTC polling.

The intervals above are scheduling targets, not guaranteed sample rates. Slow
requests extend the round; every command records its actual timing. No repeated
full discovery during healthy polling. Unsupported required inputs are explicitly
listed as missing; continue recording available inputs without estimating them.
No new semantic PID decoders are necessary: raw payloads support later analysis.

## Streaming file and boundaries

Add a small internal recording package for versioned JSONL events and bounded file
writing. Keep connection, commands, discovery and scheduling in the probe layer,
reusing existing helpers. Introduce a command observer/streaming mode so the
recorder does not grow Report.Commands or an in-memory trip history indefinitely.
The existing finite probe must retain its complete command log.

Each event has schema version, monotonic sequence number, host UTC timestamp,
elapsed monotonic nanoseconds from recording start, and connection segment ID.
Metadata identifies Europe/Moscow for human display, source, transport, adapter,
software build revision when available, timeout/retry/schedule configuration and
the owner-reported 1.5L engine displacement with explicit provenance.

Event types:

- recording_start: configuration and data/calibration limitations.
- segment_start/capabilities: transport metadata, identity/protocol and current
  PID support evidence after initialization/discovery.
- command: command, exact raw response after existing default VIN redaction,
  status/error, request/response wall timestamps, monotonic start/end offsets
  and latency; decoded value/unit/extras only when decoding is valid.
  Raw-only, NO DATA, malformed payloads and missing values stay distinguishable.
- gap/recovery/segment_end: loss of usable communication, wait/reopen events and
  explicit boundary; never carry numeric values across a gap or reconnection.
- recording_end: clean/failed reason, counts, duration and recording coverage.

Host response time is not ECU capture time. Values from separate requests are
not one simultaneous snapshot. Keep successful historical samples immutable even
if a later session fails. Record RPM=0 as a measured value, without inferring
fuel cut or automatically ending the recording.

Write complete JSON lines incrementally, with no trip-sized memory buffer; flush
each event and sync at least every 5s and on normal close. On disk/write/sync
failure stop further ECU commands and close transport; surface cleanup errors.
Reserve bounded space for final events when enforcing the file-size limit.
An abrupt power loss may leave an incomplete last line and unsynced recent data;
earlier complete lines remain independently readable. Never claim lossless power
failure recovery. No automatic log upload or raw-file commit.

## Failure and stop behavior

Preserve the existing prompt-recovery requirement after timeout, including reopen:
no new command until framing is synchronized, then reinitialize and rediscover.
Finite exponential backoff and total retry budget; if synchronization cannot be
established, end with the private partial log preserved. Semantic ECU responses
such as NO DATA do not become numeric zero or reset advertised support.

Cancellation uses the current context-aware transport and bounded helper cleanup.
Record an explicit interrupted in-flight response where possible, then stop the
connection and finalize the file using a bounded cleanup path. Do not assume a
new process or BLE connection clears an unfinished adapter command.

## Owner's collection guide

Provide docs/TRIP_RECORDING.md with the real built command after implementation.
Start and verify recording while parked, then let it run unattended. Do not
operate the laptop while driving; no special maneuvers are required. Keep the
MacBook powered/awake, secured and ventilated. Stop after parking. The recorder
cannot guarantee BLE range, macOS sleep behavior or endurance before testing.

For a first useful recording, include ordinary warm-up, idle, urban travel and
steady driving as they occur naturally. Keep separate notes of approximate
start/end odometer, fuel type and any unusual conditions. For fuel calibration,
record two full-tank endpoints, odometer readings and liters added at the second
fill; record all trips and intermediate fills in the interval. A single short
drive without measured fuel consumption cannot calibrate liters accurately.
Tank level is not available over the observed OBD bitmap.

## Verification and acceptance

Offline tests with fake transports/clocks/writers: capability gating and schedule
order, no burst on overrun, cancellation/duration bound, unknown/NO DATA payloads,
streamed initial discovery and historical sample preservation, bounded memory,
late prompt/reopen recovery and retry exhaustion, segment/gap evidence, JSONL
schema/timing/sequence, size bound, private exclusive-create output, write/sync
failure and CLI compatibility. Test default VIN redaction on streamed events.

Run formatting, go test ./... -count=1, go test -race ./..., make lint,
make build-obdprobe, Linux cross-build and git diff --check. Smoke-test --help
and offline fake recording. Hardware endurance remains pending a separate parked
test and then the owner's trip; do not connect to the car during implementation.
Update STATUS, README and connection/recording instructions. Commit and push the
reviewed implementation in a feature branch, excluding user test.py/test2.py and
private diagnostics. Do not mark M4 or M6 complete.
