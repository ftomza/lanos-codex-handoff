# Two short trips: fuel availability, 2026-11-06

## Finding

The missing instantaneous fuel estimate matches the recorded reason
`iat_unavailable_or_out_of_range`. All moving samples with unavailable fuel in
these two recordings have a fresh, available IAT value at or below -30 C. This
is the speed-density model's existing input validity guard, not evidence of a
missing opt-in flag or a browser-only rendering failure.

This explains the display. It does not establish why the IAT channel has these
values or diagnose a sensor, connector, wiring or ECU implementation.

All times below are Europe/Moscow (UTC+03:00). Analysis was offline; no adapter
or ECU connection was opened and no application/fuel-state settings changed.

## Recordings and integrity

Both fuel parts have valid JSON, consecutive sequence numbers, one session and
a normal session_end. Their source manifest reports no retention truncation.
The matching MIL parts are also closed.

| Fuel part | Recording interval | Rows | Recorded distance delta |
| --- | --- | ---: | ---: |
| part-0000000000000004.jsonl | 09:22:48.909–09:35:19.837 | 738 | 1.6704 km |
| part-0000000000000005.jsonl | 09:52:43.972–09:57:42.497 | 293 | 1.6713 km |

SHA-256:

- Part 4: `5d7521d4764f637baaf724ee9bdcb7e70ee432afab5e0e65fe4894754064be0f`.
- Part 5: `dcd10bc515010233140dfde6f071c314a5ece0323affeebeae8d9dfd1ebf4b73`.

## First trip

- Initial IAT was -40 C; early readings also included -39..-35 C. Initial ECT
  was +1 C. Cold-start was not owner-annotated, so no cold-start diagnosis follows.
- At 09:28:44.529 IAT became -6 C and the backend immediately produced fuel flow
  about 1.55 L/h. IAT alternated between -6 and -7 C while speed remained zero.
- At 09:29:38.229 IAT returned to -40 C; the next observation stopped the estimate.
  The valid running-flow interval was about 53 seconds, entirely at standstill.
- Recorded motion was 09:31:56.472–09:34:57.378. All 175 moving snapshots had
  IAT=-40 C and unavailable flow. Thus there was no valid moving L/100 km value.
- The later zero-flow interval came from fresh RPM=0; it does not validate IAT.

## Second trip

- Initial IAT was -40 C, briefly -36 C and then -40 C again. Flow was unavailable
  with the same IAT validity reason despite live RPM/MAP/speed.
- At 09:55:51.449 IAT became +8 C. At the same timestamp fuel flow became
  available: 8.1033 L/h, 18.0073 L/100 km at 45 km/h. This is one instantaneous
  point, not the trip average or a calibrated measurement.
- Subsequent fresh IAT was +8..+10 C. Running-flow samples continued until
  09:57:29.043; the later RPM=0 interval then yielded zero flow.
- Recorded motion was 09:53:59.890–09:57:00.238. Of 169 moving snapshots, 101
  lacked fuel (-40/-36 C) and 68 had fuel (+8..+10 C).

## Freshness, configuration and accounting

The IAT TTL is 20 s. Distinct-capture intervals were approximately 8.7 s median,
at most 10.73 s / 9.81 s in the respective parts. Every blocked moving sample
has `signal_ages.iat_c.applicable=true` and signal status `available`: the
temperature range, rather than expiry of those readings, blocked the estimate.

The live speed-density opt-in was enabled after model selection, as recorded
by fuel.settings.unverified_iat=true. Initial false values occurred before a
model was selected. There is no `unverified_iat` blocking reason in either trip.
VE=0.75, displacement=1.5 L, AFR=14.7, density=745 g/L and factor=1 were unchanged.

The current estimator rejects running-engine IAT <=-30 C and >100 C. The
browser shows a dash when the backend flow is unavailable; the saved reason
matches the observed display. Passing that range guard only enables an
experimental estimate; it does not verify the temperature or fuel accuracy.

The existing offline analyzer reports:

| Part | Estimated liters over covered intervals | Jointly covered distance | Time coverage |
| --- | ---: | ---: | ---: |
| 4 | 0.02306 L, predominantly standstill | 0 km | 8.06% |
| 5 | 0.08506 L | 0.9551 km | 35.10% |

Part 5's covered average is 8.906 L/100 km. Neither that average nor liters
divided by the whole recorded distance is a reliable full-trip result. Unknown
fuel intervals have not been reconstructed or filled with an assumed IAT.

## MIL context and evidence limits

Both recordings initially report MIL OFF and an available empty Mode03 reply.
There is no recorded MIL ON event or automatic Mode02/0A job. These empty lists
do not diagnose the IAT channel or establish vehicle health.

The MIL history also contains UNKNOWN/OFF_AFTER_GAP events during otherwise
active telemetry, including very small and one negative recorded MIL age.
The owner confirms the physical lamp remained off. All 181 distinct successful
MIL readings across both parts report OFF/count0; no ON incident was recorded.
There are 11 intermediate stale UNKNOWN events and two final NO DATA UNKNOWN
events. These are not recorded lamp activations.

The timestamp-ordering defect was subsequently reproduced and fixed in Capture.
At 09:31:11.433739 the timer was checking a MIL response timestamped
09:31:11.434998, about 1.26ms later than the timer's saved time. A source
publication can run between the server sampling a tick time and Capture
acquiring its lock. An old tick then evaluates newer evidence as future/stale,
and creates UNKNOWN; the next distinct OFF response becomes OFF_AFTER_GAP.
The same mechanism affects core receipt times even when MIL age is positive.

Capture now samples live observation time under its lock and ignores timer
timestamps preceding the last source observation. Genuine future-dated input,
normal source/MIL expiry, NO DATA and real gaps retain the original freshness
checks. Historical files are unchanged; the fix prevents this reproduced
ordering error on subsequent runs. It does not establish a Bluetooth disconnect
from these old rows or change the recorded cause of missing fuel in motion.

Fuel snapshots retain decoded IAT and its timestamps, not the raw 010F reply.
The next useful check is a separately authorized read-only recording of raw
010F alongside RPM/MAP/ECT, with comparison to an independent scanner or
temperature evidence. If raw replies confirm the same jumps, investigate the
IAT sensor circuit/connector and ECU interpretation. No component replacement
or fault cause is established by these files alone.

## Verification

- Streamed both private fuel parts; checked JSON, sequence, footer, SHA-256,
  distinct signal captures, model settings and all blocked moving samples.
- Ran `bin/fuelanalyze --input=data/fuel-history/live` separately with session
  `63fb0a152b402a442687b1097b78e076` and `13a1a915078abd1b55317389ab206177`.
- Checked the estimator range guard and browser handling; focused fuel and
  browser tests passed. Fuel code/settings and private input files are unchanged.
- Follow-up MIL regression tests reproduced stale-tick loss for newer MIL and
  core-only publications and a misclassified ON transition. The Capture fix
  passed those tests plus real-expiry/future-input/NO DATA controls, full Go
  tests, focused race tests and lint/vet. This is a timestamp-ordering correction,
  not new physical acceptance of automatic ON snapshots or Mode02/0A/ATH1.
- Machine-readable aggregate: `/private/tmp/lanos-fuel-availability-20261106.json`.
