# Android M5.6 LIVE journal implementation plan

**Goal** Record and export foreground LIVE sessions for diagnosing gaps.

**Spec** [LIVE journal design](../specs/2026-11-24-android-m56-journal-design.md)

- [x] Write JVM tests for rate limiting, transition and coverage events,
  path validation, rotation, backlog and truncated-tail export.
- [x] Implement the JSONL archive and native journal. Use private app storage,
  bounded telemetry submission and monotonic timestamps.
- [x] Connect native LIVE lifecycle and fuel accounting. Add a gesture-gated
  journal screen and JSONL export through the Android document picker.
- [x] Run JVM/JS/Go tests, lint and signed APK build. Verify the document
  picker, session lifecycle and journal output offline. Update project status.

## Missing prompt recovery

- [x] Reproduce the journal sequence in fake GATT tests. Exercise wake,
  reconnect, a late old prompt, acknowledged identity, timeout and ambiguous ATI.
- [x] Gate one adapter identity probe per uncertain episode and journal
  the outcome. Preserve prompt gating and the foreground lifecycle.
- [x] Verify tests and the signed APK, then document the parked test.
