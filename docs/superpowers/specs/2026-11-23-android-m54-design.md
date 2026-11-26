# Android M5.4 telemetry and recovery

M5.3 worked on the Teclast P85. M5.4 adds foreground recovery and the
supported inputs needed for the fuel computer. Keep the existing Kotlin
GATT connection, serialized ELM queue, local WebView and simulator.

Use the Go LIVE policy as the reference. Poll RPM, MAP and speed first and
slower inputs when due. Add MAP (0B), IAT (0F), short trim (06) and long
trim (07) from the 0100 bitmap. Preserve independently timestamped values.
A completed unavailable or invalid reply clears its old value. The
adapter-supply reading remains independent of ECU replies.

Enter ECU standby after an unavailable initial bitmap or three complete
failed core rounds. A valid zero or another valid core signal prevents
this transition. Clear ECU readings and capabilities on entry. Continue
voltage samples and check 0100 no faster than once every 10 seconds.
A valid new bitmap resumes polling on the same link. After three negative
sparse checks, repeat the bounded BLE ELM initialization while synchronized.

The LIVE source retries recoverable link failures after closing the old
link. A short initial burst gives way to 10-second idle waits. Clamp the
burst until a connection has supplied stable readings. Cancel the loop
when switching to the simulator or leaving the foreground. Preserve the
existing finite behavior of the adapter-only ATI check. Permission and
protocol failures remain visible.

If a command times out on an open link, wait for its outstanding prompt and
write completion without sending anything. A valid recovery starts a new
initialization. An unresolved prompt remains fenced across link and process
replacement. The dashboard distinguishes missing adapter, waiting for ECU,
synchronization, and stale readings. These states do not measure ignition
position. Physical ignition and BLE recovery remain separate acceptance tests.
