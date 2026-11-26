# Android MIL diagnostics

Use the existing foreground ELM session to poll advertised PID 0101 about
every five seconds. Show fresh ECU state and count. Invalidate on expiry,
negative/invalid reply, disconnect or new generation. Distinguish initial
ON, OFF→ON, ON→OFF and a reading after a gap. Only confirmed ON starts
paired read-only Mode 03 and 07 queries, interleaved with normal support
requests after core rounds. Bound pending diagnostic work and preserve each
mode's independent result, including empty, unsupported and interrupted.

Extend the existing Android LIVE JSONL with MIL state and incident markers.
Reference continuous one-second samples in the same session, including part
rotation, from up to 60 s before the trigger through 30 s after it. Store a
closing marker for temporal coverage or session interruption; signal validity
remains per sample. No raw ECU replies, additional connection, freeze-frame
queries, code clearing or format changes to the Go archives.

## Offline DTC descriptions

Open a read-only detail dialog from the LIVE Check Engine badge. Show
the lamp state and the independent Mode 03 and 07 replies for the
current ON episode. Preserve receipt times. Clear the current episode
on expiry, disconnect, a new generation or a confirmed OFF. A
successful empty reply is distinct from pending, unsupported,
interrupted and failed reads.

Show recorded replies at the corresponding trip-history marker, joined
by incident ID and connection generation across JSONL parts. If the
trigger is missing, show the replies as a recorded diagnostic event.
Use a small bundled Russian catalog for both views. Keep the literal
code and an explicit fallback for codes outside the catalog. The
descriptions explain what the ECU reported. Opening details uses the
already collected data and sends no requests.
