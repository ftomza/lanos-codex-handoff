# Implementation plan for Codex

## M0 — Bootstrap
- Initialize Go module and directory structure: `cmd/dashboard`, `cmd/obdprobe`, `internal/telemetry`, `internal/sim`, `internal/elm327`, `internal/serial`, `internal/server`, `web`.
- README with build/run/test commands, configuration, and architecture overview.
- Add basic CI-compatible `go test ./...` and `go vet ./...`.
Acceptance: builds and tests locally without car, adapter, or network.

## M1 — Domain + simulator
- Telemetry snapshot: RPM, speed km/h, coolant °C, adapter voltage V, each value nullable/availability-aware, source, captured_at, received_at, health status.
- Deterministic simulator with realistic idle/acceleration/deceleration and intermittent disconnect simulation.
- Source interface (`Start`, `Read`/subscription, `Close`) with context cancellation.
Acceptance: simulator generates believable values and handles cancellation; tests cover state and missing data.

## M2 — Local server + dashboard
- HTTP server serving static web UI and WebSocket or SSE telemetry feed (prefer SSE unless bidirectional transport is justified).
- Dashboard 1280×800 landscape, large speed/RPM, coolant, voltage, status and warnings; responsive smaller widths.
- Show `SIMULATOR` vs `LIVE`, stale telemetry, last update, disconnected state; avoid implying simulated readings are real.
- No cloud required. Keep CPU/memory low for Android Go device.
Acceptance: open browser on MacBook, simulator dashboard updates live and survives reconnect.

## M3 — Read-only ELM327 diagnostic CLI
- Enumerate `/dev/cu.*` ports on macOS; allow explicit port/baud (38400 and 9600 configurable).
- Robust CR-delimited ELM command framing, prompt `>`, timeout, echo handling, multi-line responses, `NO DATA`, `UNABLE TO CONNECT`, `STOPPED`, `?`.
- Initialize with ATZ, ATE0, ATL0, ATS0, ATI, ATSP0; read ATDPN, 0100, 010C, 0105, 010D, 03. Commands must be individually logged.
- Decode Mode 01 PIDs: 0C RPM=(256*A+B)/4, 0D speed=A, 05 coolant=A-40; parse DTC response 43 safely.
- Provide raw diagnostic log export with optional VIN/redaction, no automatic clearing.
Acceptance: unit tests using fixture responses, including malformed/timeouts; useful output if no SPP port exists.

M3 follow-up completed 2026-10-24: native Go BLE via tinygo.org/x/bluetooth
v0.16.0, Go 1.27, isolated macOS helper, shared ELM/probe, both transports and
private reports. One physical Go BLE baseline passed all 12 commands on the
parked running car; details/limits in STATUS.md. At that stage the dashboard remained simulator-only; M4 implementation is recorded below.

M3 trip-recorder follow-up implemented 2026-10-24: explicit --record mode,
private streaming JSONL, supported-input polling and bounded recovery for future
fuel-computer research. Offline tests passed; owner logs on 2026-10-28 cover a
60s parked session and 14m36s drive. IAT validity and failed cleanup on stop
remain unresolved; longer endurance/reconnect need testing. See TRIP_RECORDING.md
and capabilities.md. This does not complete M4 or M6.

## M4 — Live source integration
- Reuse the native BLE transport completed in the M3 utility follow-up as the priority MacBook path, with Serial/SPP as an alternative. Observed IOS-Vlink channels and one successful physical Go BLE baseline are in capabilities.md. Android still requires a separate implementation/validation decision.
- Preserve the existing context-aware byte transport, common ELM parser/framing, command allowlist and PID bitmap decoding. Notifications need not match response boundaries. Require valid channels and acknowledged subscribe/write; do not silently change write mode. The selected API does not separately expose remote property bits.
- Connect serial ELM source to same telemetry interface as simulator; adaptive polling (RPM/speed higher priority, coolant/voltage lower), per-PID capability detection.
- Exponential backoff reconnect, stale-data transitions, error metrics; never fabricate values.
- Do not assume ISO 9141/14230 support means generic PID availability. Distinguish transport failure from ECU protocol failure.
Acceptance: live Source tests pass; measure polling rate, latency and stale transitions. Existing utility transport tests and one Go BLE baseline do not verify live Source integration, Android, long-duration stability or engine-restart recovery; test those explicitly.

M4 implementation completed offline on 2026-10-28 in feat/live-dashboard:
shared continuous Stream/native Source, explicit BLE/serial CLI, four existing
instruments with independent signal freshness and source-aware UI, bounded
recovery/error counters and HTTP retained after adapter exhaustion. Go, race,
HTTP/SSE, browser-logic and build checks passed. **Physical acceptance remains
pending**: actual polling/latency, forced disconnect/stale/reconnect, longer
endurance and Android tests are not inferred from existing recorder sessions.
Build/launch and the next parked test are documented in NEXT_CONNECTION.md.
M6 fuel estimation is implemented below, with separate physical accuracy acceptance.

## M5 — Android path
- First validate browser-based UI on Teclast P85 over LAN; assess fullscreen/kiosk, sleep/wake, brightness, thermal/power behavior.
- Choose WebView wrapper or Kotlin native app based on measured performance and Bluetooth support. For direct SPP on Android, native BluetoothSocket and BLUETOOTH_CONNECT permission are needed; web browser cannot directly use Classic SPP.
- If using WebView, plan a native Android bridge for direct adapter access; a Go backend on MacBook is not a practical permanent in-car dependency.
Acceptance: documented architecture decision and Android test checklist.

## Hardware test checklist
1. With ignition on and car safely parked, use the observed IOS-Vlink BLE profile with obdprobe --transport=ble after make build-obdprobe. Classic SPP and /dev/cu.* enumeration remain an alternative.
2. If port exists, read `ATI`, `ATDPN`, `0100`; then `010C`, `0105`, `010D`; capture raw output.
3. Read `03` and preserve DTCs; never clear without explicit separate request.
4. If port absent, report macOS transport limitation; try compatible Android or USB K-Line interface before concluding ECU is unsupported.
5. If adapter answers AT but ECU does not answer Mode 01, identify ECU and investigate vendor-specific K-Line protocol; do not guess unsupported commands.

## M6 — Fuel computer (simulation first)
- Add fuel-flow estimation strategies behind an interface: direct ECU fuel-rate PID when actually supported, MAF-based estimation, speed-density using MAP/IAT/RPM with calibrated volumetric efficiency, and injector pulse-width strategy only with verified injector characteristics and ECU telemetry. Do not assume MR-140 exposes any specific PID.
- Track units and provenance (measured vs estimated), confidence/availability, timestamps, calibration factor, and engine-off/fuel-cut behavior. MAF stoichiometric estimates are approximations, especially during enrichment, deceleration, and open-loop operation.
- Calculate instantaneous consumption in L/h and L/100 km only when speed is reliably above a configurable low-speed threshold; at idle/low speed show L/h instead of dividing by near-zero speed.
- Integrate consumed liters and traveled kilometers using monotonic elapsed time and valid samples. Persist since-reset trip totals and reset timestamp across restarts, with explicit user-initiated reset; never silently reset on engine restart.
- Compute average L/100 km as 100 * cumulative liters / cumulative kilometers only when distance is meaningful. Expose fuel used, distance since reset, average, instantaneous and estimator status in API and dashboard.
- Handle stale/disconnected samples, gaps, missing PIDs, unrealistic spikes, clock changes and process restart without inventing fuel use or distance. Report incomplete coverage of trips.
- Support user calibration from measured full-to-full refueling, with validation and a reversible calibration factor; avoid claiming fuel figures are exact.
- Provide deterministic simulator fixtures and unit/integration tests for idle, motion, reset, persistence, missing data, disconnects, calibration and zero-speed edge cases.
Acceptance: fuel computer works end-to-end in simulator with persisted resettable trip totals and transparent estimated/unknown states. Live accuracy and ECU PID support remain pending hardware verification. Implement after M3/M4 as appropriate; no ECU writes.

M6 implementation completed offline on 2026-10-28 in feat/fuel-computer:
three strategies, explicit assumptions/live IAT opt-in, bounded monotonic trip
accounting and matched-interval average, source-specific atomic persistence,
coverage, revisioned reset and reversible full-to-full calibration, HTTP/SSE and
fuel dashboard panel. Injector model remains unavailable without verified data.
Simulator HTTP/SSE/reset/persistence/restart passed; full verification is recorded
in STATUS.md. Real IAT/VE/consumption accuracy, new browser visual acceptance and
Android remain pending. See FUEL_COMPUTER.md and NEXT_CONNECTION.md.

## Scope decision — OBD-only first (2026-10-21)
- Current scope is read-only data obtainable through Vgate iCar Pro BLE 4.0 Dual and the vehicle ECU (model corrected by the owner on 2026-10-24); do not implement ESP32, vehicle wiring taps, auxiliary sensors, or body-signal hardware.
- Updated on 2026-10-24: prioritize the confirmed BLE GATT path and retain Serial/SPP. Actual IOS-Vlink UUIDs and ECU responses are in capabilities.md. M3 utility now implements native Go BLE and SPP with one ELM parser/framer; one physical Go BLE baseline passed. M4 will reuse it for the dashboard; do not assume Android support or concurrent Classic/BLE operation.
- Discovery phase: record supported Mode 01 PID bitmaps (0100, 0120, 0140 etc. only where indicated), supported DTC/identification modes, protocol and raw responses. Distinguish adapter voltage from ECU-reported battery voltage.
- Prioritize RPM, speed, coolant temperature, voltage and DTC; then MAP, IAT, throttle, load, oxygen-sensor data, trims; explore fuel rate/injector timing/ECU identifiers only when supported and documented. Avoid indiscriminate proprietary requests or any write commands.
- Expose per-signal capability and freshness: supported, unsupported, not-yet-tested, temporarily-unavailable. Do not conflate these states.
- Turn signals, lights, parking brake, fuel level and other body signals are out of scope unless proven available via read-only diagnostic responses; do not infer or fabricate them.
- Fuel computer (M6) remains in scope with estimator provenance, calibration and persistent since-reset totals; disable calculation gracefully when input telemetry is insufficient.
- Hardware compatibility and available PIDs are unknown until physical testing. Document raw observations and update capability matrix afterward.

## ADAS vision track — A0–A6 (parallel research, not part of OBD-only vehicle integration)
- See [docs/ADAS.md](ADAS.md) for detailed architecture, milestones, tests, safety constraints and Codex starting task.
- USB UVC camera + MacBook M3 for development; Python/OpenCV vision service feeds read-only events to existing Go dashboard. Start with offline recorded video and A0 capture/replay tests.
- A1 lane marking detection; A2 traffic sign recognition; A3 object detection/tracking; A4 experimental proximity/TTC; A5 dashboard fusion; A6 deployment feasibility.
- The OBD-only scope decision applies to vehicle telemetry and wiring. ADAS is a separate optical sensor research track; no vehicle control, no ECU writes, no production driver alerts.
