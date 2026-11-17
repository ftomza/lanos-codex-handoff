# Ignition-cycle and Mac sleep recovery, 2026-11-11

All times below are Europe/Moscow. Analysis used saved local recordings only;
no vehicle connection, ECU commands, clearing or fuel-state changes.

## Findings

Two different recovery failures occurred. The owner confirms the engine was
running again before the first dashboard restart, and that the MacBook lid was
closed / the host slept during the second session.

| Session | Fuel part / session | MIL part / session | Recorded interval |
| --- | --- | --- | --- |
| First, parked | c / `7712183bae080e75e0adf842a3d7da63` | 8 / `044d4ddbfca442ffd957e51d455dfa89` | 09:00:41–09:05:14 |
| Second, drive | d / `5e814d0d72c7b8359c0290e6db72357d` | 9 / `fc07de51a73a22e1dc60552708d225aa` | 09:05:17–09:31:25 |

### First session: adapter alive, ECU standby does not recover

- Fuel line10: 09:00:48.566, initial unavailable0100 enters ECU standby.
  Successful ATI replies are visible at09:00:58 and09:01:13. A successful
  capability0100 at09:01:18.939 leads to generation2 and connected09:01:30.861.
  Initial standby recovery therefore worked in this recording.
- Line177: last available RPM is0 at09:03:41.108, saved09:03:41.619; speed0.
  Line181:09:03:44.934, three failed core rounds lead to ECU standby.
- ATI remains successful at09:04:05.305,09:04:46.749 and09:05:07.479, with
  latency21–34ms. Sampled0100 replies remain temporarily_unavailable through
  09:05:07.807. These archives retain sampled poll status, not every raw0100
  response, so the precise ECU/ELM reply must not be guessed as NO DATA.
- The owner confirms the engine was running again but the dashboard needed a
  restart. Session1 closes09:05:14.812; session2 starts09:05:17.190 and obtains
  connected data09:05:30.387, approximately13.2s after startup.

The recorded ATI evidence distinguishes this from a total BLE outage. Existing
`internal/probe/standby.go:108` waitECU repeats ATI/0100 indefinitely until a
valid bitmap arrives; it has no escalation to bounded reinitialization for an
adapter that answers while its ECU session remains unusable. Restart runs the
initialization/discovery path again. This identifies the recovery gap; it does
not prove whether the underlying cause is retained ELM protocol state, ECU
timing, adapter behavior or another K-Line condition. The recording does not
timestamp the owner's actual ignition-on action.

### Second session: sleep, native BLE failure and terminal source stop

| Moscow time | Evidence from fuel part d |
| --- | --- |
| 09:18:48.460 | Last captured positive RPM440.25; line791 saved09:18:48.702 |
| 09:18:56.185 | Last captured available RPM0; final successful ECU evidence |
| 09:19:01.166 | Line804: ECU standby, generation1, commands3176/errors16 |
| 09:19:02.202 → 09:24:15.203 | Lines805→806:313.001s wall interval, only1.334s elapsed; difference311.667s |
| 09:24:25.834 | Line815: ATI timeout/context deadline exceeded, measured latency3.000s |
| 09:24:26.870 | Line816: waiting for prompt synchronization |
| 09:24:35.836 | Line824: generation2 begins recovery |
| 09:24:42.838 | Line830: gap/disconnected, recoveries1 |
| 09:24:43.867 | Line831: terminal source failure, native Write/EnableNotifications timeouts |
| Through 09:31:25 | Same terminal message; commands3177, errors17, gaps2, recoveries1 remain unchanged |

The terminal message preserves:

```text
ble protocol: native BLE: timeout on Write()
timeout on EnableNotifications
native helper exit: exit status 1; timeout on Write()
timeout on EnableNotifications
```

The owner's sleep confirmation supplies context for the first clock divergence.
A second divergence at lines932→933,09:26:38.618→09:28:48.242, is129.624s wall
versus1.250s elapsed (difference128.374s); another0.584s difference appears
at958→959. These are separate from command latency and cannot be interpreted as
continuous ECU polling or combined with monotonic fuel durations. The owner has
not separately confirmed the second pause's cause.

Code tracing explains why automatic recovery ends:

1. Native Write returns the library error without a typed transient mapping
   (`internal/blehelper/native_darwin.go:137`). Native Close attempts notification
   unsubscribe and disconnect, retaining both errors.
2. `internal/blehelper/finish.go:26` emits a retryable error code only with clean
   cleanup and recognized disconnected/unavailable causes. The recorded compound
   timeout has no such classification and the helper exits nonzero.
3. `internal/ble/process.go:122` preserves the remote error and helper exit.
   `internal/probe/standby.go:53` terminates continuous Stream for an unrecognized
   or cleanup failure. `internal/live/source.go:88` publishes the terminal
   restart-required state. HTTP/recording remain alive, so later saved rows are
   not evidence of more BLE attempts.

This is a gap in recovery for the observed sleep-related native failure, not
proof of a permanently defective adapter. Simply increasing --retries cannot
change a terminal classification. Existing tests intentionally enforce that
generic cleanup/permission/protocol errors are not hidden as transient.

## Other observations

- First recording is stationary (all saved speeds0). Second records8.8909km,
  maximum76km/h. Existing fuelanalyze estimates0.84787L and9.5348L/100km on
  covered intervals, coverage71.47% including the post-drive unavailable tail.
  This is experimental model output, not measured fuel accuracy; host sleep
  is not counted as fully observed elapsed time. First recording coverage47.61%.
- Both start with MIL alreadyON/count1 and stored/pendingP0420. Mode0A remains
  unsupported, freeze-frame causeP0420. Each has one MIL_ALREADY_ON and one
  MIL_UNKNOWN after source evidence expires; no confirmed OFF transition or
  new OFF→ON event is present. Physical lamp state was not established here.
- Distinct available saved0101 captures:24 and150, allON/count1. IAT values in
  these two sessions are respectively2°C and1..7°C; no-40 reading appears in
  the retained fuel snapshots. This does not validate the sensor or fuel model.

## Next repair scope

Treat the two cases independently with offline regressions before code changes:

- Add bounded escalation out of repeated ECU-unavailable standby: reinitialize/
  rediscover without requiring the owner to restart dashboard, preserving one
  command/session at a time, prompt synchronization and sparse retry limits.
- Add explicit recovery semantics for the observed native timeout plus failed
  unsubscribe after host sleep. Preserve cleanup evidence and distinguish
  permissions/malformed IPC from transport loss. A helper must be fully reaped
  before another is launched; do not mark every joined cleanup failure retryable.
- Keep stale values hidden, generation boundaries explicit and fuel totals
  persistent. A subsequent separately authorized parked test must cover both
  ignition cycles with the Mac awake and Mac sleep/wake as distinct scenarios.

No application fix or new hardware acceptance is claimed in this investigation.

## Verification

All four selected files have valid JSON/version/source, consecutive sequences,
one session each, nondecreasing elapsed counters and normal closed footers.
Both manifests retain today's sessions; retention_truncated concerns the wider
archive and does not erase today's rows. Source file hashes/mtimes/sizes were
checked unchanged. Reports contain no structural reader issues except archive
retention in the MIL reports; incident context remains explicitly incomplete.

Offline CLI commands, all exit0 (each also accepts --json):

```sh
./bin/fuelanalyze --input=data/fuel-history/live --session=7712183bae080e75e0adf842a3d7da63
./bin/fuelanalyze --input=data/fuel-history/live --session=5e814d0d72c7b8359c0290e6db72357d
./bin/milanalyze --input=data/mil-events/live --session=044d4ddbfca442ffd957e51d455dfa89
./bin/milanalyze --input=data/mil-events/live --session=fc07de51a73a22e1dc60552708d225aa
```

Focused existing recovery/cleanup tests pass:

```sh
go test ./internal/probe ./internal/blehelper ./internal/ble -run 'Test(StreamWaitsForECU|StreamDetectsIgnitionOff|StreamRetriesZero|StreamFatal|StreamTimeout|StreamReconnectCleanup|HelperSeparatesTransient|HelperJoinedFatal|ReportedDisconnect|HelperCloseFailure)' -count=1
```

These are existing synthetic checks, not a new regression proving a repair of
today's native timeout. Full Go/race/web suites were not rerun for documentation
and read-only analysis. Application source and owner scripts remain untouched.
Local generated reports: `/private/tmp/lanos-20261111-review.6SwpEc`.

| Input | SHA-256 |
| --- | --- |
| Fuel c | `7cfc41d3c099ddd8853e6b9e171c674f2e6320552cdbad404c6ba7cd76efdb3c` |
| Fuel d | `58b3a6fc6ca622b55af17ce68dc35cdbf1b5388b1e80023ad98ad6921398bcef` |
| MIL8 | `ca62e88c9dbe290134ba4fa6fa9afdf95da0e216553db338f681b2b58d24c829` |
| MIL9 | `c39fd3ade90077fdf531e2ff7fb25b9f8e534d8e12541e6b6d29876e9d4b95fe` |
