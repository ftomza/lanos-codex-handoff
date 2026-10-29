# Pending DTC read (Mode 07)

Owner requested reading pending codes after a parked, steady-MIL/running-engine
baseline returned MIL on and no stored Mode 03 codes. Scope approved by
«читаем» on 2026-10-29; no repeated hardware/design approval is needed.

Use a minimal permanent extension to the tested Go obdprobe rather than a
separate raw-transport script or a wider diagnostic scanner. Add --read-pending
to finite probes only, after the existing stored-code read. Add only literal 07
to the read-only allowlist; Mode 04/write/clear/VIN/arbitrary commands stay blocked.
Mode 0A/permanent codes are outside this request. Dashboard/recording/default
commands remain unchanged. No dependency/Go/state-schema changes.

Decode 47 responses with the same strict pair/padding/duplicate checks as 43;
wrong mode, malformed/negative/NO DATA responses never mean an empty success.
Optional pending_dtcs has source=ecu_mode07, status, codes, capture time and raw
response. Retry reset/invalidation cannot expose stale pending codes as current.
A framed unsupported/NO DATA reply is partial, with preserved raw evidence;
transport timeout follows the existing prompt recovery/retry policy.

Plan: failing parser/allowlist tests; failing finite-probe opt-in/default/error/
staleness and CLI-mode tests; implement and run offline Go/race/lint/build checks;
check no concurrent OBD process, perform one --retries=0 read while the owner is
parked with the engine running, preserve a new private report, explain observed
codes without asserting a component diagnosis, and update STATUS/NEXT_CONNECTION.
No clearing, ignition change, proprietary requests or live dashboard launch.


Outcome: implemented and tested offline; one physical read on 2026-10-29
returned a valid empty 47 response while MIL remained on and 43 was also empty.
The cause remains unknown. Private full report and exact safe fixture paths,
commands and next steps are recorded in STATUS.md.
