# Android saved-trip history plan

**Spec** [Android history](../specs/2026-11-25-android-trip-history-design.md)

- [x] Add JVM tests for legacy samples, multiparts, resets, missing data
  and bounded thinning. Implement a bounded report over the archive reader.
- [x] Reuse the dashboard chart renderer in a local Android history screen.
  Keep the system export action and test the historical graph input.
- [x] Build and sign the APK, run JVM, web and Go checks, and exercise the
  history screen with saved parts on Android without opening LIVE.

## Empty recordings

- [x] Test startup-only recordings, including rotations and a valid
  interrupted tail. Preserve zero-valued readings, diagnostics,
  broken archives and candidates changed after preview.
- [x] Add the native cleanup action with confirmation, serialized archive
  access, and refreshed report and export lists.
- [x] Build, sign and test the APK. Verify the P85 preview and cancellation
  in SIM. Use fixtures to verify deletion.
