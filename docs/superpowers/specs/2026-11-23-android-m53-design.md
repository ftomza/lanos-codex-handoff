# Android M5.3 live telemetry

The Teclast P85 has passed M5.2 with IOS-Vlink. M5.3 adds an explicit LIVE
choice to the dashboard. It shows RPM and coolant from the ECU and supply
voltage from the adapter. Speed is polled when PID 0D is advertised. The
separate ATI check and simulator remain available. The screen identifies
the selected source during acquisition, failures and loss of freshness.
Leaving the foreground closes GATT. Tap the live action to reconnect.

Use the proven native Android GATT link and one serialized ELM327 command
session shared by the ATI check and LIVE. The Go BLE path uses ATE0, ATL0, ATI, ATSP0, 0100 then ATDPN. The Android port
uses the same order and does not reset the adapter or change its headers.
Use the 0100 bitmap before polling 010C, 0105 and 010D. ATRV is independent
of the bitmap. The allowed command list is closed and typed. A command
advances only after its write acknowledgement and the ELM prompt. Invalid
framing retires the connection. A timed-out or interrupted command requires
recovering the prompt before any new write. The app holds one foreground
session lease and persists a conservative busy marker during the session.
After a lost prompt the user can clear the marker after confirming a physical
power cycle of the adapter.

The bounded frame parser handles echo, carriage return and linefeed,
SEARCHING, negative or missing responses, multiple identical replies and
malformed bytes. RPM and speed expire after three seconds. Coolant and
adapter supply expire after ten seconds. Unavailable, unsupported, invalid
and stale signals have no value. Adapter voltage can remain usable when
the ECU sends no data. Host timestamps describe actual response receipt.

Parked hardware acceptance requires the three requested readings on the
P85 in LIVE. The first build is verified offline with the Go capture
fixture, fake GATT sessions and the shared browser display model.
