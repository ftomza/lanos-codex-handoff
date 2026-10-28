# M6 fuel computer and dashboard

Status: proposed for owner review, 2026-10-28. No implementation or new hardware
session yet. Branch feat/fuel-computer, based on c5c674e (live dashboard).

## Outcome and boundaries

Implement M6 from PLAN.md end-to-end in the simulator and expose a transparent
experimental live estimate where inputs allow it. Preserve the existing four
instruments, simulator/live identity, independent freshness and single adapter
connection. Add instantaneous L/h or L/100 km, estimated consumed liters, since-
reset distance, covered-interval average, coverage and explicit model status.
Persist trip totals and calibration locally; engine/process restart must not
silently reset them. Provide explicit reset and reversible full-to-full
calibration. No new dependencies or Go version changes.

No physical access is authorized by this implementation request. No ECU writes,
clear commands, VIN/proprietary requests, injector characteristics or fuel-cut
heuristics. The computer changes local state only. It does not infer tank level,
fuel remaining, range, fuel cost, or a correct measured consumption from AI-92
and rounded odometer endpoints. Preserve owner scripts and private recordings.

## Alternatives and recommendation

1. Recommended: a source-independent Go fuel domain, three interchangeable
   strategies, persistent trip accounting, dashboard/API controls and simulator
   fixtures. Existing live telemetry adds already queried MAP/IAT and supported
   MAF/fuel-rate metadata. Experimental speed-density is explicitly enabled for
   live operation; ordinary LIVE startup remains honest when IAT is unverified.
2. Simulator-only fuel computer: fewer live assumptions, but the requested real
   dashboard would show only unavailable fuel until a later integration task.
3. Require a direct ECU fuel-rate reading: potentially better input, but PID 5E
   and MAF 10 are not advertised by this vehicle's observed bitmap. Do not poll
   absent PIDs or promise that an adapter change will expose them.

Implement option 1. The simulator acceptance is complete offline; live numerical
accuracy and calibration require separate evidence and remain experimental.

## Models and explicit assumptions

Declare a small Estimator interface in internal/fuel. Results carry value,
method, provenance, confidence, availability/reason, input timestamp horizon and
validity deadline. Strategies operate on validated immutable telemetry and
never initialize transports or store history. Selection order in auto mode:

- Direct Mode 01 PID 5E, only when advertised and a fresh decoded response exists:
  ECU-reported L/h, provenance ecu_reported, accuracy still unverified.
- MAF PID 10, only when advertised and fresh: approximate gasoline consumption
  from air mass / assumed air-fuel ratio / nominal fuel density.
- Speed-density using fresh RPM/MAP/IAT: approximate four-stroke airflow and
  gasoline consumption. Initial VE is an explicitly documented assumption,
  not a fitted map or a claim of fuel accuracy.

Select one method per source capability generation. A temporarily missing input
makes that method unavailable; do not silently change methods for each sample.
A different method/configuration starts a new accounting boundary and requires
an explicit local trip reset before accumulating incompatible model totals.
Direct flow, MAF and speed-density receive deterministic tests even though only
the last currently has the required PID support on the real Lanos. Injector
pulse-width remains unavailable without a verified signal and injector data.

Nominal gasoline settings: owner-reported 1.5 L four-stroke engine, AFR 14.7,
density 745 g/L, constant VE 0.75. AI-92 does not verify actual ethanol content,
density or mixture. Validate overrides and record effective settings in API/state.
Do not apply O2/trims as an automatic fuel-flow correction or infer DFCO from
throttle/RPM. Enrichment, fuel cut, temperature and VE variation remain model
limitations, even after a scalar calibration.

Use SI conversions explicitly:

```
T_K = IAT_C + 273.15
rho_kg_m3 = MAP_kPa * 1000 / (287.05 * T_K)
volume_m3_s = displacement_L / 1000 * RPM / 120 * VE
air_g_s = rho_kg_m3 * volume_m3_s * 1000
fuel_L_h = air_g_s * 3600 / (AFR * density_g_L)
```

MAF supplies air_g_s directly. RPM=0 only establishes zero flow for a currently
fresh stopped engine; stale/missing RPM never becomes zero. A valid direct fuel
rate remains an ECU-reported value. Unknown engine-off/fuel-cut behavior is not
replaced with guessed fuel use.

## IAT and live availability

Current logs contain -40 C and later constant +20 C. Decoder matches bytes, but
sensor validity is unverified. Default live speed-density status is unavailable
with reason unverified_iat; require `--fuel-allow-unverified-iat` to show a low-
confidence experimental estimate from the actual fresh ECU IAT. This flag does
not validate the sensor and must be visible in the API/UI. Never silently
substitute +20 C, coolant or an ambient weather temperature.

Model operating bounds exclude IAT <= -30 C or >100 C, MAP outside (0, 120] kPa,
RPM outside [0, 7000] and non-finite inputs. Out-of-range means outside this
model's accepted operating range, not a sensor diagnosis. The opt-in cannot
bypass these bounds or freshness. Constant +20 C remains labelled unverified.
Mass airflow/fuel-rate and derived-flow bounds are likewise validated against
explicit configuration; implausible samples are unavailable, never clamped
into plausible numbers. Live remains estimated/low confidence after calibration.

## Telemetry and sampling

Extend internal/live signal definitions with MAP (3s), IAT (20s), MAF (3s) and
fuel-rate (3s) metadata. Preserve the existing four flat fields and TTLs. The
shared stream already polls MAP and IAT; add MAF/fuel-rate to a core schedule
only when advertised. Do not send 0110/015E on the observed Lanos bitmap.
Expose a generation/segment identifier so lost intermediate latest-only gap
messages cannot bridge an accounting interval across reconnection.

Extend the deterministic simulator with explicit synthetic MAP/IAT/flow inputs
and engine-off/idle/motion/deceleration/missing-input fixtures. Simulator estimates
and state remain separate from live; no synthetic input enters a live snapshot.
Fuel accounting runs once in the backend, independent of browser count or SSE
frequency. A new internal/fuel service consumes source updates and uses a 250ms
monotonic tick, sharing the latest validated snapshot. Repeated tick/update
callbacks at the same time cannot double-count an interval.

At each accounting boundary validate both endpoints, global source silence,
per-input freshness and source generation. Use monotonic elapsed duration, not
serialized wall-clock subtraction, with bounded dt <=3s and no extrapolation
past a measurement's expiry. Integrate validated endpoint rates/speeds using
trapezoids; discard any invalid intervening interval and break the anchor.
No integration across disconnect, unavailable input, generation change, process
restart, backwards time, oversized dt or impossible samples. Persisted values
never reconstruct a monotonic anchor after restart. UTC wall timestamps describe
reset/save/receipt only. Tests inject a clock; do not change simulator timing
just to recover a monotonic component from its existing UTC timestamps.

## Totals and coverage

Track independently:

- distance_km from valid speed intervals;
- base_liters from valid fuel intervals before scalar calibration;
- covered_distance_km from intervals where both fuel and speed are valid;
- observed/covered/gap elapsed duration, incomplete flag and explicit reasons.

Fuel may accumulate during valid idle intervals. Distance can accumulate while
fuel is unavailable, but must not dilute the displayed covered-interval average.
Average is 100 * calibrated liters from jointly covered intervals /
covered_distance_km, and is null until covered distance >=0.1 km. Keep jointly
covered liters separate from fuel-only idle/missing-speed intervals so the
numerator and denominator always refer to the same intervals. Label average
as partial when coverage is incomplete; it is not full-trip fuel consumption.
At reliably measured speed >=5 km/h show instantaneous 100 * L/h / km/h; below
that threshold show L/h. Missing speed must not become an idle assertion: show
L/h with speed unavailable and no L/100 km. Threshold is configurable/validated.

No silent reset on an engine restart. After an existing trip is resumed by a new
process, mark the unobserved interval as unknown/incomplete; do not invent its
distance, fuel or duration. A coverage percentage concerns observed recording
time only and is not a promise of full-trip coverage. Historic totals can remain
visible when live measurements expire, with since-reset and partial labels;
instantaneous values expire with their own input horizon in API and browser.

## Persistence and calibration

Use stdlib-only versioned JSON state, default data/fuel/sim.json or
data/fuel/live.json (add /data/fuel/ to .gitignore), overridable by --fuel-state.
Store source/model fingerprint, raw and jointly covered totals, calibration,
reset/save times, coverage and revision. No instantaneous readings, monotonic
anchors or command history persisted. Separate simulator/live files and reject
source/configuration mismatches before hardware starts; provide an explicit
reset path. A newly discovered method mismatch pauses fuel accounting until
reset, while the original four live instruments remain available.

Validate version, all numeric bounds, timestamps and complete JSON. Never
silently replace corrupted state with a zero trip. Create private 0700 directory
and 0600 file; write temp + sync + atomic rename. Checkpoint at most every 5s,
on successful local mutations and graceful shutdown. A crash may lose up to the
checkpoint interval; do not claim otherwise. Prevent concurrent writers using
an exclusive sidecar lock; if present, fail with actionable guidance. Never
automatically remove an unknown lock or overwrite an unreadable state.
Fail initial state/lock errors before opening hardware. Runtime persistence
failure is visible as unsaved/persistence_error, retains last committed state
and must not claim successful reset/calibration. Preserve the real error.

Reset is a local user action with a confirmation dialog; it creates a new trip
revision/reset time and clears totals/coverage anchors. Preserve calibration
unless the user explicitly chooses to restore factor 1. State is durable before
a reset success is reported.

Full-to-full calibration accepts measured gasoline liters corresponding exactly
to the current since-reset interval, with enough distance and valid coverage.
Suggest factor = measured_liters / base_liters; validate finite positive inputs,
minimum 10 covered km and bounds [0.25, 4] for the resulting factor. Reject
incomplete/mixed-model intervals; missing recording cannot be calibrated away.
User explicitly confirms matching full-to-full boundaries. Preview the old/new
factor and adjusted totals before saving. Save the previous factor for undo and
provide restore-to-1. Raw totals never change; factor applies consistently to
historical/current estimated liters. A scalar factor is not a validated VE map.
Do not claim the existing 167→177 km trip supplies measured fuel liters.

## API, controls and dashboard

Add a fuel object to snapshot/SSE containing nullable instantaneous L/h and
L/100 km, units/mode, method/provenance/confidence/reason, own validity horizon,
estimated liters, total/covered distance, covered average, coverage, reset time,
calibration and persistence status/revision. Existing API fields remain intact.
Keep computation outside HTTP serialization; browser count never affects totals.

Add one compact fuel panel below the existing instruments: instantaneous value,
covered average, estimated liters and distance after reset, model/coverage note,
and reset/calibration controls. Keep the 1280x800 layout usable and responsive
on narrow screens. Low-speed display switches units without division by zero.
UNKNOWN/connecting/stale/disconnected hide instantaneous values; saved totals
are clearly historical and retain incomplete/unsaved labels. Every synthetic
estimate stays SIMULATOR; live speed-density shows estimate/uncalibrated or
adjusted factor, plus unverified-IAT warning when opted in. No tank/range widget.

Local JSON POST endpoints implement reset, calibration preview/apply/undo/default.
Mutations require same-origin requests, application/json, a small bounded body,
strict field/number validation and expected state revision; reject cross-origin
or stale changes. No permissive CORS. UI submits only after explicit user action.
Existing LAN deployment remains a trusted-local-network setup, not authenticated
remote access. These endpoints never send an ECU command or restart hardware.

CLI: keep current sim/live commands working; `--fuel=auto` by default and `--fuel=off`
for disabling the module. Add validated model settings, low-speed threshold,
state path and explicit live IAT opt-in. `--help` and invalid configuration have
no state/hardware side effects. Document exact live experimental command and
unavailable reasons. No change to obdprobe --record format or owner log files.

## Verification and delivery

Execute inline with TDD, no delegation. Meaningful domain tests cover formulas
and units, selection, stale/missing/non-finite/out-of-range inputs, idle, engine
off, no inferred fuel cut, low-speed thresholds, real-zero values, monotonic
integration, duplicate timestamps, clock changes, reconnect generations, gaps,
matching average intervals and coverage. Test persistence atomic/error/corrupt/
source mismatch/locking/restart behavior; test calibration validation,
reversibility and reset revision/durability.

Use saved response fixtures and synthetic labelled data; no real car access.
Go HTTP/SSE tests cover one computer for multiple clients, source freshness,
controls/revisions/origin checks and persisted restart. Node tests cover expiry,
unit switching, model/partial labels and missing values. Smoke simulator HTTP/SSE
and shutdown without devices; inspect layout at 1280x800 and a narrow viewport
if browser tooling is available, otherwise document visual testing as pending.

Run gofmt/import review, go test ./... -count=1, go test -race ./..., make lint,
make test-web, make build-dashboard, make build-obdprobe, Linux cross-build,
native-helper vet and git diff --check. Preserve CLI/record/live regressions.
Update README, PLAN/PROJECT/STATUS, next-connection guide and fuel documentation.
Commit/push feat/fuel-computer with required trailer; exclude owner scripts and
private logs/state. M6 simulator acceptance can be completed offline; physical
fuel accuracy, IAT validation and full-to-full calibration remain pending.
