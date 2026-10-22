# ADAS Vision — architecture and delivery plan

Status: planned research prototype; NOT a safety-rated driver assistance system.
Owner: Codex implementation, reviewed against recorded-video tests.
Hardware: USB UVC forward-facing camera; MacBook M3 (macOS) for initial inference; Teclast P85 is display target only, not presumed capable of running vision inference.

## Goals and exclusions
- Detect lane markings and estimate ego-lane geometry; identify potential lane departure only when calibration and tracking confidence are adequate.
- Detect and classify selected traffic signs (initially speed limits, STOP, yield) with temporal confirmation and unknown state; avoid assuming sign applies to the ego lane without contextual validation.
- Detect/track cars, motorcycles, cyclists and pedestrians ahead; investigate approaching-object risk.
- Read-only and advisory: no steering, braking, throttle, ECU writes or vehicle-control integration. Keep original dashboard and driver attention primary.
- Initial development and validation on prerecorded video and stationary camera only. No reliance on prototype warnings for driving decisions or live-road safety.

## Proposed components
```
USB UVC camera / prerecorded MP4
  -> Video Capture (AVFoundation/OpenCV, frame timestamps)
  -> Frame Preprocessing (resize, exposure, optional undistortion)
  -> Vision pipeline
       Lane Detector + Temporal Lane Tracker
       Sign Detector + Sign Classifier + Temporal Voting
       Object Detector + Multi-object Tracker
       Experimental Relative-Motion / TTC Estimator
  -> Event Aggregator (confidence, hysteresis, TTL, source, calibration state)
  -> Local JSON event API / SSE (versioned schema)
  -> Go dashboard adapter -> UI overlay / experimental notifications
OBD speed (when supported) -> optional fusion, never required for video tests
```
Python 3 + OpenCV + ONNX Runtime or PyTorch on macOS for research iteration; evaluate Core ML conversion only after establishing baseline. Keep inference process isolated from Go dashboard. Avoid adding Python runtime into existing Go telemetry packages. Prefer localhost SSE or bounded IPC; no cloud requirement.

## Suggested repository structure
```
adas/
  README.md
  pyproject.toml
  src/lanos_adas/{capture,lanes,signs,objects,tracking,risk,events,api}.py
  tests/
  fixtures/README.md
  configs/default.yaml
docs/ADAS.md
docs/ADAS_EVAL.md
```
Model weights and road recordings are NOT committed; document license, origin, hashes and reproducible acquisition instructions. Respect third-party dataset/model licenses and privacy (faces, plates, location).

## Milestones
### A0 — Capture, replay, benchmark
- Enumerate USB cameras; support --source camera:index and --source video:path.
- Save timestamped video/metadata; fixed deterministic replay mode, frame drops and disconnect simulation.
- Overlay capture FPS, inference latency, frame age and pipeline health.
Acceptance: repeatable 720p video playback, graceful missing camera and corrupted frames, no unbounded queue/memory.

### A1 — Lane perception
- Baseline with classical OpenCV ROI/edge/line fitting or segmentation model; compare under curves, shadows, worn markings, intersections and missing lanes.
- Temporal smoothing, camera calibration and confidence; suppress departure warnings when lane geometry or ego pose is unreliable.
Acceptance: annotated fixtures, metrics for marking detection and false departure events; lane absence yields UNKNOWN.

### A2 — Traffic sign recognition
- Baseline detector + classifier with region-specific label map; speed-limit/STOP/yield; deduplicate via track and multi-frame voting.
- Distinguish detected sign from legally applicable limit (different lanes, ramps, temporary signs).
Acceptance: precision/recall per class on held-out clips, sign persistence and expiration tests, UNKNOWN when ambiguous.

### A3 — Object detection and tracking
- Lightweight detector with documented model weights/license; track vehicles, pedestrians, cyclists.
- Define road corridor/ego path gating; track ID persistence and occlusion behavior.
Acceptance: precision/recall, tracking consistency, false-positive review, crowded/low-light fixtures.

### A4 — Experimental proximity / TTC
- Estimate image-based closing cues, optical expansion and (where justified) monocular road-plane geometry; never claim camera-derived metric distance is measured truth.
- TTC is only defined when closing motion, trajectory intersection and calibration confidence are sufficient; otherwise UNKNOWN.
- Log candidate events in offline shadow mode; no driver-facing collision alert until extensive independent validation and safety review.
Acceptance: synthetic known-motion tests, sensitivity analysis, negative examples (adjacent lanes, stationary roadside objects, turns), abstention on unreliable scenes.

### A5 — Dashboard integration
- Versioned event contract: event_id, kind, captured_at, processed_at, source, confidence, severity, state, ttl_ms, metadata and calibration/version identifiers.
- Integrate Go SSE/dashboard with source provenance, latency and stale/unknown states. Optional OBD speed fusion only after verified PID support.
- Experimental overlay clearly marked ADAS DEMO; visual-only offline replay first. No distracting alerts while driving.
Acceptance: deterministic end-to-end video replay -> events -> dashboard; disconnect, stale and version compatibility tests.

### A6 — Deployment study
- Benchmark Mac M3 performance and memory/power; decide whether a dedicated automotive-capable compute unit is needed for eventual in-car deployment.
- Teclast P85 can display events over LAN; do not assume Android Go can infer at target FPS.
Acceptance: written latency budget, performance report, mounting/calibration plan, thermal/power risk register and explicit go/no-go decision.

## Evaluation and quality gates
- Split videos by drive/location/date to prevent near-duplicate leakage. Include daylight, dusk/night, rain, glare, roadworks, curves, worn/absent markings, mixed signs, vulnerable road users and camera shake.
- Log input frame time, output time, p50/p95/p99 end-to-end latency, achieved FPS, dropped frames, CPU/GPU and memory.
- Define baseline acceptance thresholds from measured validation set, not arbitrary safety claims; report precision, recall, false alarms per hour, missed critical events and confidence calibration for each feature.
- Replay determinism: identical fixtures/config/model produce same event sequence within documented numeric tolerance.
- Unit tests for stale-frame rejection, TTL, track expiry, temporal voting, missing frames, low-confidence suppression, and no unsafe commands.
- Any prospective real-road validation requires independent safety review and non-reliance on prototype alerts; first trials should be recorded-video or controlled/off-road.
- Prototype must fail silent/UNKNOWN on lost camera, low confidence, missing calibration or excessive processing lag.

## Codex starting task
Implement A0 only. Create isolated Python module, camera/video capture, deterministic replay, metrics and tests. Add docs and update docs/STATUS.md. Do not download heavyweight model weights or enable real-time driver alerts. Leave A1–A6 as planned.
