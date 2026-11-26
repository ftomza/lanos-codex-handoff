# Android MIL implementation plan

**Goal** Add current Check Engine status and automatic read-only incident recording.

**Spec** [Android MIL](../specs/2026-11-25-android-mil-design.md)

- [x] Verify the received Vgate fixture and malformed/negative replies with
  Kotlin tests. Implement MIL TTL, transitions and bounded 03/07 results.
- [x] Verify capability gating, serialized scheduling and core polling with
  JVM tests. Add diagnostic commands to the existing Android allowlist.
- [x] Verify JSONL context and interruption markers, and the WebView badge
  on expired, disconnected and SIM inputs. Keep the journal schema and
  include the supported trim signals.
- [x] Build and verify the APK. Run Android/Go/JS regression and verify the
  tablet layout offline. Update the project status and verification steps.

## Offline DTC descriptions

- [x] Test and expose current-episode native replies without duplicating
  them in journal samples.
- [x] Test and join archived replies across rotation and preserve replies
  whose trigger is missing. Verify isolation by ID and generation.
- [x] Test the two-mode view, including empty/error distinction, unknown
  codes and freshness. Reuse the catalog and renderer in both screens.
- [x] Build and verify the APK, check both screens with offline fixtures,
  and update the documentation and status.
