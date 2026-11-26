package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class TripHistoryTest {
    @get:Rule val folder = TemporaryFolder()
    private val start = Instant.parse("2026-11-25T08:00:00Z")
    private val id = "5cc5f996-a2d8-45cc-b469-c218c73297c9"

    @Test fun groupsRotatedLegacyPartsAndCountsOnlyCompatibleIntervals() {
        val archive = TripArchive(folder.root)
        part(archive, 1, 0, listOf(sample(0, 4.0, 20.0, 19.9, 1), sample(1000, 4.1, 20.5, 20.4, 1)), "rotation", 1000)
        part(archive, 2, 1000, listOf(
            sample(2000, 4.2, 21.0, 20.9, 1),
            event(2200, "mil_transition").put("kind", "MIL_ON"),
            sample(3000, 5.0, 30.0, 29.9, 2),
            sample(4000, 5.1, 30.5, 30.4, 2),
        ), "screen_off", 4000)

        val history = TripHistory(archive)
        val ref = history.catalog().single()
        assertEquals(2, ref.parts.size)
        val report = history.report(ref)
        assertEquals(1.5, report.getDouble("distance_km"), 1e-9)
        assertEquals(0.3, report.getDouble("liters"), 1e-9)
        assertEquals(20.0, report.getDouble("average_l100"), 1e-9)
        assertEquals(75.0, report.getDouble("coverage_percent"), 1e-9)
        assertEquals(1, report.getJSONArray("markers").length())
        val points = report.getJSONObject("series").getJSONArray("rpm")
        assertTrue(points.getJSONObject(3).getBoolean("break"))
    }

    @Test fun missingPartsAndCorruptRowsBreakTheGraphAndNeverBridgeTotals() {
        val archive = TripArchive(folder.root)
        part(archive, 1, 0, listOf(sample(0, 0.0, 0.0, 0.0, 1), sample(1000, 0.1, 0.5, 0.5, 1)), "rotation", 1000)
        part(archive, 3, 1000, listOf(
            sample(2000, 2.0, 20.0, 10.0, 1),
            null,
            sample(3000, 2.1, 21.0, 11.0, 1),
            sample(4000, 2.2, 22.0, 12.0, 1, rpm = null),
            sample(5000, 2.3, 23.0, 13.0, 1),
        ), null, 5000)
        val history = TripHistory(archive)
        val report = history.report(history.catalog().single())
        assertEquals(2.5, report.getDouble("distance_km"), 1e-9)
        assertTrue(report.getBoolean("incomplete"))
        assertEquals(1, report.getInt("bad_rows"))
        val points = report.getJSONObject("series").getJSONArray("rpm")
        assertEquals(5, points.length())
        assertTrue(points.getJSONObject(2).getBoolean("break"))
        assertTrue(points.getJSONObject(3).getBoolean("break"))
        assertTrue(points.getJSONObject(4).getBoolean("break"))
    }

    @Test fun downsamplingPreservesPeakAndRecordedGap() {
        val archive = TripArchive(folder.root)
        val rows = (0L..1500L).map { sec ->
            sample(sec * 1000, sec / 3600.0, 0.0, 0.0, 1,
                rpm = if (sec == 400L) null else if (sec == 321L) 6100.0 else 850.0)
        }
        part(archive, 1, 0, rows, "background", 1_500_000)
        val history = TripHistory(archive)
        val report = history.report(history.catalog().single())
        val points = report.getJSONObject("series").getJSONArray("rpm")
        val entries = (0 until points.length()).map(points::getJSONObject)
        assertTrue(entries.size <= 1024)
        assertTrue(report.getBoolean("downsampled"))
        assertTrue(entries.any { it.getDouble("value") == 6100.0 })
        assertTrue(entries.drop(1).any { it.getBoolean("break") })
        assertEquals(1501, report.getInt("samples"))
    }

    @Test fun missingPrefixKeepsElapsedOriginAndDoesNotCountTowardCoverage() {
        val archive = TripArchive(folder.root)
        part(archive, 2, 8000, listOf(
            sample(9000, 0.9, 9.0, 9.0, 1), sample(10_000, 1.0, 10.0, 10.0, 1),
        ), "background", 10_000)
        val history = TripHistory(archive)
        val report = history.report(history.catalog().single())
        assertEquals(start.toString(), report.getString("from"))
        assertEquals(10_000, report.getInt("duration_ms"))
        assertEquals(10.0, report.getDouble("coverage_percent"), 1e-9)
        assertTrue(report.getBoolean("incomplete"))
    }

    @Test fun calibrationBreaksFlowAndLateMilSurvivesMarkerLimit() {
        val archive = TripArchive(folder.root)
        val before = sample(0, 0.0, 0.0, 0.0, 1)
        val after = sample(1000, 0.1, 0.5, 0.5, 1)
        after.getJSONObject("fuel").put("factor", 2).put("flow_lph", 2.0)
        val events = (1L..256L).map { event(it, "coverage_resumed") }
        part(archive, 1, 0, listOf(before) + events + listOf(
            event(999, "mil_transition").put("kind", "MIL_ON"), after,
        ), "background", 1000)
        val history = TripHistory(archive)
        val report = history.report(history.catalog().single())
        val series = report.getJSONObject("series")
        assertTrue(series.getJSONArray("flow_lph").getJSONObject(1).getBoolean("break"))
        assertFalse(series.getJSONArray("rpm").getJSONObject(1).getBoolean("break"))
        assertEquals(1, report.getInt("omitted_markers"))
        val markers = report.getJSONArray("markers")
        assertEquals(256, markers.length())
        assertEquals("mil_on", markers.getJSONObject(255).getString("kind"))
    }

    @Test fun historyMatchesDtcByIncidentAndGenerationAcrossRotatedParts() {
        val archive = TripArchive(folder.root)
        val incident = "a58d239a-cc32-4762-9689-1b74a6f97bc8"
        fun reply(tick: Long, generation: Int, command: String, vararg codes: String) =
            event(tick, "mil_dtc").put("incident_id", incident).put("generation", generation)
                .put("command", command).put("status", "available").put("codes", org.json.JSONArray(codes.toList()))
                .put("captured_at", start.plusMillis(tick).toString())
        val lamp = event(500, "mil_transition").put("kind", "MIL_ON").put("incident_id", incident).put("generation", 1)
        part(archive, 1, 0, listOf(lamp), "rotation", 1000)
        part(archive, 2, 1000, listOf(
            reply(1000, 1, "03", "P0420"),
            reply(1100, 1, "07"),
            reply(1200, 2, "07", "P0300"),
            reply(1300, 2, "03", "bad-code"),
        ), "background", 1400)

        val history = TripHistory(archive)
        val markers = history.report(history.catalog().single()).getJSONArray("markers")
        assertEquals(2, markers.length())
        val paired = markers.getJSONObject(0).getJSONObject("dtc_replies")
        assertEquals("P0420", paired.getJSONObject("03").getJSONArray("codes").getString(0))
        assertEquals(start.plusMillis(1000).toString(), paired.getJSONObject("03").getString("captured_at"))
        assertEquals(0, paired.getJSONObject("07").getJSONArray("codes").length())
        val orphan = markers.getJSONObject(1)
        assertEquals("mil_dtc_result", orphan.getString("kind"))
        assertEquals("P0300", orphan.getJSONObject("dtc_replies").getJSONObject("07").getJSONArray("codes").getString(0))
        assertEquals("invalid_response", orphan.getJSONObject("dtc_replies").getJSONObject("03").getString("status"))
    }

    private fun part(archive: TripArchive, number: Int, from: Long, rows: List<JSONObject?>, reason: String?, end: Long) {
        archive.openPart(start).bufferedWriter().use { out ->
            fun write(type: String, ms: Long, payload: JSONObject) {
                payload.put("type", type).put("schema", "lanos.android.trip").put("version", 1).put("source", "live")
                    .put("session_id", id).put("part", number).put("at", start.plusMillis(ms).toString())
                    .put("elapsed_ms", ms)
                out.appendLine(payload.toString())
            }
            write("session_start", from, JSONObject().put("started_at", start.toString()))
            for (row in rows) {
                if (row == null) out.appendLine("{damaged")
                else write(row.optString("type", "sample"), row.getLong("elapsed_ms"), row)
            }
            if (reason != null) write("session_end", end, JSONObject().put("reason", reason))
        }
    }

    private fun sample(tick: Long, liters: Double, distance: Double, covered: Double, generation: Int, rpm: Double? = 850.0): JSONObject {
        val fuel = JSONObject().put("status", "available").put("flow_lph", 1.0)
            .put("liters", liters).put("distance_km", distance).put("covered_km", covered)
            .put("average_l100", if (covered > 0) liters * 100 / covered else JSONObject.NULL)
            .put("revision", 1).put("factor", 1)
        val coverage = JSONObject().put("reset_at", start.toString())
            .put("observed_ms", tick).put("covered_ms", tick).put("gap_ms", 0)
        val rpmSignal = JSONObject().put("status", if (rpm == null) "stale" else "available")
            .put("value", rpm ?: JSONObject.NULL)
        return JSONObject().put("elapsed_ms", tick).put("generation", generation).put("stage", "Live")
            .put("fuel_mode", "Experimental").put("fuel", fuel).put("coverage", coverage)
            .put("signals", JSONObject().put("rpm", rpmSignal))
    }

    private fun event(ms: Long, event: String) = JSONObject().put("type", "event").put("elapsed_ms", ms).put("event", event)
}
