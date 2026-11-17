# Owner-recorded trips and P0420, 2026-11-07

All times are Europe/Moscow (UTC+03:00). This review used saved local JSONL
only; no adapter connection, ECU command, code clearing or fuel-state change.

## Main finding: confirmed ECU MIL request and P0420

The third session contains a confirmed OFF-to-ON transition. The last saved
successful OFF response was at 16:54:12.027771; the first ON response was at
16:54:18.224874. These bound detection, not the exact physical lamp-on time.
The owner has not confirmed today's physical lamp state.

| Time | Request | Saved response / interpretation |
| --- | --- | --- |
| 16:39:44.676 | 03 | `43 00 00 00 00 00 00`: available empty stored list at startup |
| 16:54:18.224 | 0101 | `41 01 81 07 E1 00`: MIL ON, reported DTC count 1 |
| 16:54:19.482 | 03 | `43 04 20 00 00 00 00`: stored P0420 |
| 16:54:25.508 | 07 | `47 04 20 00 00 00 00`: pending P0420 |
| 16:54:28.403 | 0A | `7F 0A 11`: unsupported service, not an empty permanent-code list |
| 16:54:31.503 | 020000 | `42 00 00 7E 38 00 00`: frame 00 bitmap |
| 16:54:34.434 | 020200 | `42 02 00 04 20`: freeze-frame cause P0420 |

Raw replies also include CR delimiters and the ELM prompt in the private files.
P0420 means catalyst system efficiency below threshold, bank 1. It does not
alone establish a failed catalyst or justify replacement.

The ECU freeze frame reports RPM 925.25, speed 0 km/h, ECT 85 C, MAP 44 kPa,
load 4.3137%, STFT -1.5625% and LTFT -11.71875%. These are stored ECU context;
they are not simultaneous host measurements at MIL detection. Frame PID 03 is
preserved as raw_only (`42 03 00 01 00`); no new interpretation is applied.
At host detection the live snapshot instead reports RPM 827, speed 0, ECT 85,
MAP 44 and IAT 13 C. ATRV 13.1 V is an older adapter-supply reading, not a
measurement at the alternator terminals.

Catalyst readiness changes from incomplete before detection to complete at ON.
Complete means the monitor ran, not that it passed. Last saved successful ON
was 16:55:11.772361. UNKNOWN at 16:55:27.646 reflects expired evidence (MIL age
15.873 s, TTL 15 s); there is no confirmed subsequent ON-to-OFF transition.

## Diagnostic incident completeness

MIL part 5 contains `incident-000000000000000c`, with 53 pre-event frames
(16:53:17.901–16:54:18.225) and 31 post-event frames
(16:54:19.327–16:54:48.290). The saved flags are
`pre_window_retained=true`, `post_window_retained=false`, `had_gaps=true`,
ending with `window_elapsed`. Some individual inputs expire even while frame
effective health is connected. This is useful context, not complete continuous
90-second telemetry. Later freeze-frame replies remain saved as independent
diagnostic rows, including RPM/speed after the post window closes.

This owner recording provides physical-response evidence for the automatic
ON-triggered stored/pending/frame00 workflow and unsupported Mode0A handling.
It does not complete the separate M3.3.7 acceptance checklist, prove ignition
recovery/endurance, identify the ECU model, or validate Android/ATH1.

## Fuel and input availability

Four fuel parts represent three sessions: parts 7/8 are one rotated session.
Existing offline `fuelanalyze` output:

| Recording | Parts | Observed distance | Estimated covered fuel | Covered average | Time coverage |
| --- | --- | ---: | ---: | ---: | ---: |
| 15:05:38–15:15:11 | 6 | 3.0342 km | 0.22486 L | 12.119 L/100 km | 47.23% |
| 15:26:03–16:09:46 | 7/8 | 20.9959 km | 2.13377 L | 10.267 L/100 km | 97.30% |
| 16:39:19–16:56:16 | 9 | 8.6779 km | 0.67248 L | 8.240 L/100 km | 75.29% |

All three analyzer reports are incomplete. Covered averages use matched valid
intervals; dividing covered liters by the entire observed distance would be
misleading. These remain experimental speed-density estimates, not measured
fuel consumption or a calibration result. No missing fuel was reconstructed.
Part 9 raw cumulative deltas (8.68719 km / 0.674066 L) differ slightly from the
analyzer's eligible-interval totals above. Its separately sampled integral is
0.58846 L, indicating limitations of sparse/irregular saved samples; it is not
independent evidence of actual fuel accuracy.

- First session: IAT -40..+6 C, with 27 of 58 distinct captures at -40. All
  114 moving snapshots without fuel are blocked by IAT validity. Fuel first
  appears at 15:08:44 with +2 C, drops at 15:10:59 with -40, then returns at
  15:12:38 with +4, with further brief glitches.
- Second session: IAT mainly +7..+23 C, but four distinct -40 readings remain.
  The main fuel gap is around 15:27:21–15:27:57. This is the best-covered trip,
  approximately 10.3 L/100 km; it still does not validate the IAT channel or VE.
- Third session: numeric IAT captures are +10..+29 C. All 30 snapshots blocked
  specifically by `iat_unavailable_or_out_of_range` have IAT capture age
  20.005..28.966 s, beyond TTL 20 s, and `applicable=false`. Stored signal status
  remains available, but freshness rejects it. Here IAT expiry explains that
  reason, rather than an out-of-range temperature. RPM/MAP/source expiry also
  causes missing fuel.

In session 3, deduplicated saved last-poll samples show latency worsening after
16:46: median 0.255 to 0.337 s, maximum 0.388 to 6.367 s. This is a sampled
subset, not every command or proof of a transport root cause. The degradation
starts before MIL detection, so it cannot all be attributed to the subsequent
diagnostic job. Final counters show 4 errors, 2 gaps and 2 recoveries.

Owner clarification after this review: during the last recording an OS process
consumed CPU and the whole host became severely unresponsive. This supplies
host-overload context for the delayed polling, expired inputs and missing fuel
estimates. No CPU profile was recorded in these files, so the contribution of
host scheduling versus other delays cannot be quantified. Do not attribute
these gaps to an adapter/ECU defect from this session. This clarification does
not invalidate the saved P0420 replies or explain the earlier -40 C IAT values.

## MIL freshness and yesterday's fix

First and second sessions have respectively 94 and 482 distinct successful
MIL observations, all OFF/count 0; both start with empty Mode03 and contain no
UNKNOWN events. Third session has 144 OFF, 10 ON and one unavailable capture.
It contains five intermediate UNKNOWN/OFF_AFTER_GAP pairs and one final UNKNOWN.
Saved MIL ages are nonnegative. Source expiry is independently meaningful even
when MIL age is below its own TTL: global capture age is 4.969 s near 16:46:18
and 3.150 s at 16:50:22, beyond source TTL 3 s. A later neighboring fuel sample
must not be used as evidence available at an earlier MIL event.

These records do not reproduce yesterday's negative-age ordering case; they
also do not justify claiming every UNKNOWN is false or declaring all freshness
and reconnect acceptance complete. The owner-reported host overload in the
third session is consistent with genuine expiry of telemetry during CPU stalls.

## Next diagnostic evidence

Preserve DTCs and this freeze frame. For the next separately authorized parked
connection, first record physical lamp state and read current MIL/stored/pending
without clearing. Investigate IAT against ambient temperature after a cold soak
and preserve raw 010F replies; sensor/wiring/ECU cause remains unconfirmed.

For P0420, check exhaust leaks, relevant fueling/ignition faults and both oxygen
sensor circuits before deciding on catalyst testing/replacement. O2 captures
around this event are asynchronous and often 14–28 seconds apart: they cannot
establish switching frequency, compare waveforms or distinguish sensor/catalyst
failure. LTFT near -12% is relevant context, not a diagnosis. There is no proven
causal link between the IAT anomalies and P0420.

## Offline verification and provenance

Fuel parts 6–9 and MIL parts 3–5 pass JSON/version/session/consecutive-sequence
and normal session_end checks; sequence continuity includes fuel rotation 7/8.
All today's sessions are retained. Fuel manifest retention_truncated=true
concerns an older retired part, not a missing portion of today's sessions;
MIL manifest reports no truncation.

Commands (all three exit 0):

```sh
./bin/fuelanalyze --input=data/fuel-history/live --session=5df1ff4b303f7c8ff3f856dbf84b2c61
./bin/fuelanalyze --input=data/fuel-history/live --session=d862e4b53b8fbbc3ba60364be6bfa506
./bin/fuelanalyze --input=data/fuel-history/live --session=d58dc28ba82fe1f999b1b1e9d15e1dd0
```

SHA-256 (private originals remain unchanged):

| Part | SHA-256 |
| --- | --- |
| Fuel 6 | `78d7327e8a89c2c2c65120956493f97441a7531e97e8395d4d0cc4fb8e63644a` |
| Fuel 7 | `6eda40d68897b42b1060d654497a4a78cc7012df1cba6ab195fd51e3f46f5f41` |
| Fuel 8 | `784e765176d886b34b530f154bfc8575f4f120cb1330da0b2d8aa60beeec5f73` |
| Fuel 9 | `3e0a9b9e67f729ce362e64584d4428456c827fd08f4a927cb62212ae30acf518` |
| MIL 3 | `56a1c6a13a086173e51971b41be4520bec50bf58df654866d7805761c25df85b` |
| MIL 4 | `12ce1c21c47b65c7f6162ad15ad972cf0f71d52df2e2ca975f7ba0d3f0e5bea8` |
| MIL 5 | `a93bc50852f1f566e24611d81809f044e5980532820268d2512613a297242fd1` |

Application code is unchanged; Go/web suites were not rerun for this analysis.
This manual review does not implement the deferred M3.3.5 correlation analyzer.
