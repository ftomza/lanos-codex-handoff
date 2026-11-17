# Ignition and host-sleep recovery repair

Owner authorized implementation on2026-11-11 after reviewing the two repair
scopes in diagnostics/2026-11-11-reconnect-review.md. Branch
fix/ignition-sleep-recovery from29c2106. No new physical connection is authorized.

## Intent and approach

Restore continuous LIVE polling after the observed ECU standby and native BLE
timeout cases, without restarting dashboard or losing fuel totals. Two local
repairs use the existing sequential Stream and isolated helper process.

Alternatives considered: restarting the entire dashboard loses its HTTP/SSE
lifecycle; retrying arbitrary error strings hides permissions/protocol faults;
increasing --retries does not affect terminal classification. Chosen builder
approach: bounded standby escalation plus narrowly typed native timeouts, with
explicit child completion before retry. No new dependency, Go change or ECU
command, no clearing, fuel reset/model/state or archive-format changes.

## ECU standby

Keep10s cancelable pauses and ATI/0100 checks. After3 consecutive negative0100
checks with a valid adapter identity, emit recovery phase ecu_reinitialize and
return to a new Stream segment on the synchronized existing transport. Run
normal initialization/discovery; do not scan DTC/all supported readings unless
0100 actually returns a valid bitmap. If still absent, repeat sparse standby.
An earlier valid0100 resumes immediately. A timeout/partial prompt still uses
Recover; no write before synchronization. Finite Run/Record remain bounded and
retain their existing loops. Freshness and generation gaps continue to prevent
integration through missing data; RPM0 with a responding ECU stays valid.

## Native BLE timeout and cleanup

tinygo.org/x/bluetooth v0.16.0 darwin returns unexported/plain errors for the
observed timeouts. Go docs were checked first, then targeted source: Write()
returns exact timeout on Write(); EnableNotifications returns exact timeout on
EnableNotifications. Recognize only these exact messages at their respective
native call boundaries (write, subscribe/unsubscribe). Preserve the error text
and operation while wrapping new blewire.ErrTimeout. Unknown/wrapped/permission
errors remain unclassified. Disconnect errors are never classified by text.

The helper may emit code timeout when its primary operation is a recognized
transient cause and cleanup succeeds or fails solely with a recognized native
timeout. It retains cleanup text in its final error event. Any unknown cleanup,
output failure, invalid IPC or unrecognized joined leaf remains fatal. A cleanup
timeout on an otherwise clean normal shutdown remains an error, not success.
Mixed recognized timeout/disconnect causes are transient; timeout takes priority.

Failed writes have no early error ack: their final error event is emitted after
cleanup, so the parent cannot race an untyped ack against the typed final cause.
Successful writes retain acknowledgements. Existing parent command deadlines
still bound pending writes; subsequent prompt recovery can observe the final
typed timeout. The parent maps code timeout to ErrTimeout and its existing Close
waits/reaps the child before a new transport opens. Only a clean helper exit
permits retry; unknown/nonzero exits remain fatal. Existing sparse retries and
prompt recovery remain unchanged. Do not broaden context.DeadlineExceeded into
unconditional reconnect or send blind ATZ before an outstanding prompt.

This narrowly supersedes the2026-10-28 rule making every cleanup failure fatal:
only explicitly recognized native notification timeout after a transient primary
failure becomes retryable, with its evidence retained. Process isolation and
reaping are the retry boundary; remote electrical/adapter state is not proven.

## Verification and limits

TDD: reproduce standby that becomes usable only after reinitialization, repeated
absence without command storms/DTC scans, cancellation, native exact timeout
mapping and rejected lookalikes, helper compound timeout/cleanup evidence and
no failed ack, unknown cleanup/output failures, parent timeout IPC and fully
reaped clean/nonzero exits, Stream retry with retries0 and late prompt safety.
Exercise Source/fuel/SSE via existing integration suites. Run full Go/race/lint,
web tests and dashboard/native helper/probe builds. Preserve private inputs and
owner scripts. Update STATUS, PLAN and next-connection documentation.

Offline verification cannot prove ignition-cycle or sleep/wake compatibility.
Next separately authorized parked acceptance: ignition off/on with Mac awake,
then sleep/wake as a separate scenario; one client/source, fresh readings and
preserved fuel totals. Adapter wake and delivery of an outstanding ELM prompt
remain physical-test questions, never bypassed with fabricated synchronization.
