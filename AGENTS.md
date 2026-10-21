# Agent instructions — Lanos Dashboard

Read `docs/PROJECT.md`, `docs/PLAN.md`, and `docs/STATUS.md` before making changes. Execute one milestone at a time. Update `docs/STATUS.md` after every milestone, documenting commands, test results, blockers and next steps. Do not claim hardware compatibility without a successful physical test.

## Engineering rules
- Go backend with clean boundaries: telemetry domain, source adapters, transport, web UI. Keep simulator and ELM327 source interchangeable.
- Prefer Go standard library; justify third-party dependencies. Unit tests for protocol framing, ELM parsing, PID decoding, stale data, and reconnection logic.
- Never send ECU write/clear commands by default. Diagnostics are read-only; DTC clearing requires a separate explicit user-approved feature, out of MVP scope.
- Never silently display stale telemetry as live. Add timestamps, freshness indicators, disconnect state, and bounded retries.
- macOS M3 Bluetooth Classic SPP may not expose a `/dev/cu.*` port. Do not assume a serial port exists; provide a probe and clear fallback guidance.
- Treat ECU as *possibly* Delphi MR-140, not confirmed; K-Line electrical compatibility does not imply standard OBD-II PID compatibility.
- Make web dashboard responsive at 1280×800 and usable on an Android 15 Go 8-inch tablet via browser/WebView. Keep a path to a native Android app later.
- Run formatting and tests; document any tests that require hardware.
