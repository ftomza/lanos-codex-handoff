# M5.5 Android Fuel Computer

The Android dashboard estimates current and trip fuel use with the Go
speed-density model. It uses live MAP, IAT, RPM and speed or the simulator's
own values. Running-engine LIVE estimates require explicit local opt-in to
unverified IAT. A fresh RPM of zero produces zero flow. All estimates are
low confidence. MAP and IAT in SIM are synthetic.

The Kotlin model keeps monotonic input validity and integrates independently
of WebView delivery. It uses a trapezoid over intervals at most 3 s and
requires valid endpoints from the same generation. Fuel and distance have
independent totals. The average uses joint coverage and appears from
0.1 covered km. Instant units change at 5 km/h. Loss of telemetry, suspend,
source switches and restarts cannot bridge a gap.

An atomic local state file holds versioned raw totals, coverage, current and
previous factors, and a revision. SIM, default LIVE and opted-in LIVE use
independent files. State saves about every 5 s, at lifecycle boundaries and
before explicit changes become visible. Incompatible or unreadable state is
reported and retained. The current estimate can still be used.

Reuse the shared fuel presentation and confirmation dialogs. Keep the readings
visible and collapse the settings on the instrument surface. An allowlisted local WebMessage request handles fuel
controls. The native opt-in confirmation is opened by a gesture-gated route.
Full-to-full preview requires confirmed matching boundaries, complete
coverage, at least 10 covered km, positive raw liters and 0.01–1000 measured
liters. Factors are limited to 0.25–4. Apply uses the exact preview and
revision. Undo, return to ×1 and confirmed trip reset are local operations.
The existing Android read-only ELM command set applies.
