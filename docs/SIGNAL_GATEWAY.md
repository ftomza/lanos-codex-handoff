# M7 — Lanos Signal Gateway

## Goal
Read additional vehicle signals unavailable through the existing generic OBD2 interface, without replacing or impairing the factory instrument cluster. Integrate a second BLE telemetry source into the Android Dashboard alongside Vgate.

## Evidence and limits
Reference T150 wiring diagram: https://www.chevyman.ru/Lanos/T150/electrics/schemes/shema-soedineniy-kombinacii-priborov
The exact cluster part number, connector pinout, electrical characteristics and equipment variant of the owner's car are unverified. The diagram is a research lead, not an approved wiring plan. Existing OBD2 observations are in docs/capabilities.md; fuel level PID 2F is not advertised.

## Candidate signals
| Signal | Proposed source |
|---|---|
| Fuel level and reserve | Sender/cluster circuit; high-impedance sensing and calibration |
| Left/right turn, hazards | Individual lamp signals; infer hazards cautiously |
| Parking lights, high beam, fog lights | Verified lighting circuits |
| Low beam | Separate verified headlamp circuit if needed |
| Parking brake, oil warning, charge warning, MIL | Candidate indicator circuits; shared brake warning must not be mislabeled |
| Driver door, ignition | Verified switch/ignition circuits |
| RPM, speed, coolant | Existing OBD2 preferred; cluster signals only if justified |

No claim that all signals exist at the cluster connector. Do not infer exact pin numbers, polarity, voltage or signal type before identification and measurement.

## Architecture
Factory harness -> passive pass-through T-harness -> factory cluster (unchanged).
High-impedance protected sensing branches -> ESP32-S3 Signal Gateway -> BLE -> Android Telemetry Coordinator.
ECU -> Vgate ELM327/KWP2000 -> separate BLE connection -> same Android coordinator.
No ECU or cluster control, no signal injection. A gateway failure or power loss must not disrupt the original instruments. Do not touch SRS, ABS or other safety-critical circuits in the initial scope.

## Preliminary hardware (not a purchase-ready BOM)
- ESP32-S3 DevKit for bench prototype.
- ADS1115 or suitable ADC, with independently designed high-impedance buffered/protected analog inputs; never connect directly to vehicle signals.
- Protected digital inputs selected after measuring polarity, loading and waveform; optocouplers only where appropriate.
- Automotive-rated power supply with fuse, reverse-polarity and transient protection; verify quiescent current and sleep/wake behavior.
- Matching male/female connectors for a reversible T-harness, no cutting factory wiring.
- Independent buzzer for lights-left-on alert, subject to power budget and verified inputs.
No direct 12 V connection to MCU/ADC. Validate electrical protection and fault isolation on bench before any vehicle connection; qualified automotive electrical review before installation.

## Software
Prefer ESP-IDF or C++/PlatformIO for firmware. Proposed layout: firmware/signal-gateway/{src,include,tests}, hardware/{schematics,pcb,pinout}, docs/.
Define a versioned BLE GATT protocol with sequence, timestamps/age, validity, reconnect and explicit unknown values. Android must keep OBD and gateway sources independent and never silently substitute stale data. Candidate fields: ignition, fuel_level_pct, fuel_reserve, turn_left, turn_right, parking_lights, low_beam, high_beam, parking_brake, oil_pressure_warning, driver_door_open. Unknown/unverified fields null. Fuel percentage is calibrated estimate, not a direct measurement; distinguish sender readings from liters and account for slosh/nonlinearity.

## Forgotten lights
First inspect existing factory buzzer and driver door switch. Proposed independent alert when ignition off AND lighting on AND driver door open. Unknown inputs must not be treated as confirmed off. Gateway must not depend on tablet or Vgate for this alert. Design low-power operation so the warning system does not itself drain the battery. No automatic switching of vehicle lights.

## Milestones
- M7.0 Identify cluster: front/rear photos, label/part number, connectors, matching wiring diagrams, evidence-backed pin map with confidence and unknowns.
- M7.1 Characterize selected signals using safe high-impedance measurements by a competent person; document voltage, polarity, waveform, loading and ignition states.
- M7.2 Bench ESP32 prototype with simulated inputs and tests.
- M7.3 Design protected input circuits and reversible harness; bench fault tests and electrical review.
- M7.4 Versioned BLE protocol, firmware tests and simulator.
- M7.5 Android integration after coordinating with feat/android-mvp; test simultaneous independent BLE connections, freshness and reconnect.
- M7.6 Fuel sender calibration and uncertainty assessment.
- M7.7 Independent forgotten-lights warning, power/sleep and battery drain tests.
- M7.8 Custom PCB in KiCad only after prototype validation.

## Codex handoff
Read AGENTS.md, docs/PROJECT.md, docs/PLAN.md, docs/STATUS.md and docs/capabilities.md. Work on feat/signal-gateway. Start only with M7.0: gather documentation and create an evidence-backed pinout template; request owner photos/part number if missing. Do not invent pinouts, design final circuit values, connect hardware or modify vehicle wiring without separate approval. Record sources, unresolved questions, acceptance criteria and next steps in docs/STATUS.md. Preserve existing Go/Android work. No hardware compatibility claim without actual tests.
