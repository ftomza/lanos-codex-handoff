package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class TripCleanupTest {
    @get:Rule val folder = TemporaryFolder()
    private val start = Instant.parse("2026-11-25T08:00:00Z")
    private val archive by lazy { TripArchive(folder.root) }
    private val cleanup by lazy { TripCleanup(archive) }

    @Test fun removesCompleteRotatedAndInterruptedSearches() {
        part(1, rows = listOf(event("connection_stage").put("stage", "Scanning"), sample()))
        part(2, rows = listOf(sample()), footer = null)
        part(3, rows = listOf(sample()), footer = "rotation")
        part(3, number = 2, offset = 1000, rows = listOf(sample()))

        val candidates = cleanup.emptySessions()
        assertEquals(setOf(id(1), id(2), id(3)), candidates.map { it.id }.toSet())
        assertEquals(4, archive.entries().size)
        assertEquals(TripCleanupResult(3, 0), cleanup.remove(candidates))
        assertTrue(archive.entries().isEmpty())
    }

    @Test fun preservesReadingsDiagnosticsAndFuelChangesAcrossParts() {
        part(1, rows = listOf(sample(0)))
        part(2, rows = listOf(sample(), event("mil_dtc")))
        part(3, rows = listOf(event("connection_stage").put("stage", "WaitingEcu")))
        part(4, rows = listOf(sample()), footer = "rotation")
        part(4, number = 2, offset = 1000, rows = listOf(sample(liters = 8.0)))
        part(5, rows = listOf(sample().apply {
            getJSONObject("mil").put("state", "off").put("dtc_count", 0)
        }))
        part(6, rows = listOf(sample().apply {
            put("signals", JSONObject().put("rpm", JSONObject().put("status", "temporarily_unavailable")))
        }))
        assertTrue(cleanup.emptySessions().isEmpty())
        assertEquals(7, archive.entries().size)
    }

    @Test fun preservesTruncatedRowsMissingPartsAndDroppedRecords() {
        val truncated = part(1, rows = listOf(sample()), footer = null)
        truncated.appendText("{\"type\":\"sample\"")
        part(2, number = 2, offset = 1000, rows = listOf(sample()))
        part(3, rows = listOf(sample()), footer = "rotation")
        part(4, rows = listOf(event("records_dropped")))
        part(5, rows = listOf(sample()), footer = "rotation")
        part(5, number = 2, offset = 1001, rows = listOf(sample()))
        assertTrue(cleanup.emptySessions().isEmpty())
        assertEquals(6, archive.entries().size)
    }

    @Test fun revalidatesAfterPreviewAndIgnoresNewCandidates() {
        val stale = part(1, rows = listOf(sample()), footer = null)
        part(2, rows = listOf(sample()))
        val preview = cleanup.emptySessions()
        part(3, rows = listOf(sample()))
        stale.appendText(row(1, 1, 999, "sample", sample(0)).toString() + "\n")

        assertEquals(TripCleanupResult(1, 1), cleanup.remove(preview))
        assertEquals(setOf(id(1), id(3)), TripHistory(archive).catalog().map { it.id }.toSet())
    }

    private fun id(session: Int) = "5cc5f996-a2d8-45cc-b469-${session.toString().padStart(12, '0')}"

    private fun part(
        session: Int, number: Int = 1, offset: Long = 0, rows: List<JSONObject>,
        footer: String? = "journal_opened",
    ): java.io.File {
        archive.openPart(start).bufferedWriter().use { out ->
            out.appendLine(row(session, number, offset, "session_start", JSONObject().put("started_at", start.toString())).toString())
            rows.forEachIndexed { index, body ->
                out.appendLine(row(session, number, offset + index + 1, body.optString("type", "sample"), body).toString())
            }
            if (footer != null) {
                out.appendLine(row(session, number, offset + 1000, "session_end", JSONObject().put("reason", footer)).toString())
            }
        }
        return folder.root.resolve(archive.entries().first())
    }

    private fun row(session: Int, number: Int, tick: Long, type: String, body: JSONObject) =
        body.put("type", type).put("elapsed_ms", tick).put("session_id", id(session)).put("part", number)
            .put("schema", "lanos.android.trip").put("version", 1).put("source", "live")

    private fun sample(rpm: Int? = null, liters: Double = 7.0): JSONObject {
        val signal = JSONObject().put("value", rpm ?: JSONObject.NULL)
            .put("status", if (rpm != null) "available" else "temporarily_unavailable")
        return JSONObject().put("stage", "Scanning").put("signals", JSONObject().put("rpm", signal))
            .put("mil", JSONObject().put("state", "unknown"))
            .put("fuel", JSONObject().put("status", "unavailable").put("liters", liters).put("flow_lph", JSONObject.NULL))
    }

    private fun event(name: String) = JSONObject().put("type", "event").put("event", name)
}
