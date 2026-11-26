# Android M5.6 LIVE journal

A foreground LIVE attempt starts a private diagnostic session. Leaving the
dashboard, selecting SIM, or opening the journal closes the session.
ECU standby, retries and recovery remain within the same session. SIM never
writes to the LIVE journal. The prior fuel totals remain independent.

Record a snapshot every second from the native dashboard tick and log
connection transitions, source generations, fuel coverage breaks and recovery
when they happen. Each record has UTC wall time and monotonic elapsed
milliseconds. Samples contain the four fuel inputs, their support and
freshness, the estimated fuel view and coverage counters. Log typed state
without display messages, raw ECU frames, adapter identifiers or location.

Use compact Android JSONL with its own schema. Append on one worker and
write a marker if the worker misses records. Flush every five seconds and on
a clean close. Rotate within a single session and keep the newest 24 parts
of at most 2 MiB each. After interruption keep the complete rows and omit an
unterminated tail on export. A missing end row indicates an interrupted file.

The **Журнал LIVE** action in fuel settings opens a local list. The user picks
a part and a destination in Android's document picker. Use the existing local,
gesture-gated navigation. The dashboard opens no background BLE connection
and sends its existing read-only requests.

## Recovery when a BLE subscription misses the previous prompt

The P85 journal from 2026-11-26 shows a repeated stall after successful
subscription, both after waking and after a foreground BLE loss. On the
new LIVE link, wait eight seconds for the old prompt. If the unfinished
command is known in this process and was different from ATI, send one
adapter-local identity request. Require successful GATT write completion
and a complete identity frame before normal initialization. Discard old
non-identity frames during this probe. Keep the probe and response bounded.

Continue passive prompt recovery for a cold-start uncertainty or an
unfinished ATI. A failed probe leaves the prompt gate closed and offers
the physical power-cycle action. Persist the probe guard before sending
ATI. An unconfirmed probe across a connection or process restart
requires a physical power-cycle confirmation. Keep journal evidence
of the attempt and outcome. The adapter-check screen retains its
passive recovery policy for normal outstanding commands.
