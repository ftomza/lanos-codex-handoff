# M5 — Android Native Dashboard MVP

## Goal
Deliver an offline, standalone Android APK for Chevrolet Lanos, connecting directly to Vgate iCar Pro BLE 4.0 Dual (IOS-Vlink) without MacBook or external Go server.

## Architecture
Kotlin + Android BluetoothGatt + local WebView assets reusing the existing dashboard. Keep a single serialized ELM327 command queue and distinct simulator/live sources. Prefer Kotlin implementation for MVP; assess Go logic reuse via shared golden fixtures rather than requiring gomobile or an embedded HTTP server. Preserve existing Go behavior.

## Hardware facts
- Target: Teclast P85, Android 15 Go, landscape 1280x800; verify actual device properties during acceptance.
- BLE service: 000018f0-0000-1000-8000-00805f9b34fb
- Notify RX: 00002af0-0000-1000-8000-00805f9b34fb
- Write TX: 00002af1-0000-1000-8000-00805f9b34fb
- ECU likely Delphi MR-140 but unconfirmed; observed ATDPN A5 (KWP2000 fast init). Read-only only.

## Milestones
### M5.1 Bootstrap — execute first, then stop
- Read AGENTS.md, README.md, docs/PROJECT.md, docs/PLAN.md, docs/STATUS.md, docs/capabilities.md, docs/FUEL_COMPUTER.md, docs/NEXT_CONNECTION.md; inspect actual source tree before implementation.
- Create Android Gradle Kotlin project under android/ with compatible installed AGP/Kotlin/SDK versions; document minSdk/targetSdk decisions.
- Embed existing HTML/CSS/JS dashboard in a secure local WebView; avoid unrestricted JavaScript interfaces and unnecessary internet access. Preserve responsive 1280x800 landscape/fullscreen.
- Implement an Android simulator feed with correct telemetry shape, stale/disconnect behavior, and explicit SIMULATOR labeling; no car required.
- Add JVM tests, build debug APK, document emulator launch and output path. Do not claim emulator or hardware tests unless run.
- Update docs/STATUS.md and docs/PLAN.md with actual evidence. Stop and report before BLE work.

### M5.2 BLE GATT — requires owner approval for physical test
- Request Android 12+ Bluetooth runtime permissions; bounded scan, connect, discover, validate GATT properties, subscribe via CCCD, write with correct mode, assemble notifications until ELM prompt >.
- Handle lifecycle, timeouts, cancellation, disconnect, bounded reconnect; no parallel OBD clients.
- Acceptance: real Android device receives ATI from IOS-Vlink.

### M5.3 ELM327 + live RPM, coolant and adapter voltage
- Reuse Go fixtures and protocol behavior: ATE0, ATL0, ATS0, ATSP0, ATI, ATDPN, 0100, 010C, 0105, 010D, ATRV; confirm exact initialization against existing implementation.
- Parse echo, CR/LF, SEARCHING, NO DATA, STOPPED, UNABLE TO CONNECT, ?, partial notifications, timeout.
- Show adapter supply voltage in volts from `ATRV` in the **Напряжение адаптера** card. Treat an invalid, missing or stale reading as unavailable.
- Acceptance: with the car parked, the dashboard shows RPM, coolant temperature and adapter supply voltage with LIVE and freshness status. It never substitutes simulator values.

### M5.4 Telemetry integration
- Capability-gated polling, adaptive schedule, per-signal timestamps, null/unsupported/stale distinctions; MAP, IAT, trims and supported inputs; ECU standby/ignition recovery, one shared transport.
- Acceptance: explicit disconnected/stale states and recovery without fake readings.

### M5.5 Fuel Computer
- Port speed-density and persistence/reset/calibration/coverage rules exactly as docs/FUEL_COMPUTER.md; IAT remains unverified and fuel remains estimated.
- Add shared Go/Kotlin golden fixtures and documented numerical tolerance.

### M5.6 Tablet hardening
- Screen sleep/wake, app background/foreground, energy behavior, storage/export JSONL, reconnect, immersive UI, permission UX, lifecycle tests. Background BLE only with compliant foreground-service design if actually needed.

## Safety and quality gates
- Read-only ECU. No Mode 04, configuration, actuator commands or implicit diagnostic writes.
- Never display stale telemetry as live, or unverified fuel as measured.
- No unapproved physical vehicle access. Do not claim Android compatibility without real hardware testing.
- Tests: Kotlin JVM/unit, fake BLE state machine, protocol fixtures, UI smoke, Go regression (go test ./..., go vet ./..., existing web tests).
- After each milestone: changed files, commands/results, limitations, run instructions, next step. Keep milestones independently reviewable.

## Immediate Codex instruction
M5.1 through M5.3 were accepted on the Teclast P85. The owner confirmed
`ATI` returned `ELM327 v2.3` from IOS-Vlink and that the dashboard showed
live RPM, coolant temperature and adapter supply voltage. On the M5.4 build
the owner also confirmed live readings returned after an ignition cycle.
The owner has seen an M5.5 LIVE estimate after opting in to unverified IAT.
Accuracy and coverage remain unverified. The first M5.6 increment adds the
[Android LIVE journal and JSONL export](../android/README.md#журнал-live-и-экспорт-jsonl).
The remaining M5.6 work covers the physical sleep, ignition and BLE recovery checks
and tablet hardening informed by those recordings.
