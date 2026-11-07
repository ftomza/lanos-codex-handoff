# Fixture provenance

Tests construct synthetic private temporary archives. All envelopes, timestamps,
sensor values, event ordering and non-hardware scenarios are synthetic.

Three raw packets in TestAnalysisSeparatesStoredEvidenceAndLateDiagnostics are
verbatim observations from the owner's 2026-11-07 MIL recording:

- Mode03: `43 04 20 00 00 00 00 \r\r>` — P0420.
- Mode0A: `7F 0A 11 \r\r>` — unsupported service.
- Mode02 frame00 PID02: `42 02 00 04 20 \r\r>` — P0420 cause.

Only raw bytes and their confirmed decoding are reused. Their test timestamps
and surrounding records do not claim physical timing or acceptance. The full
private JSONL files remain ignored and unchanged. Reference analysis:
`docs/diagnostics/2026-11-07-trip-review.md`.
