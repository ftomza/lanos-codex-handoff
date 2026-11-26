package io.github.ftomza.lanos

import java.io.File
import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class LiveMilTest {
    private val base = Instant.parse("2026-11-25T08:00:00Z")

    @Test fun decodesRealVgateOnWithEmptyStoredAndPendingReplies() {
        val fixture = JSONObject(File(checkNotNull(System.getProperty("lanos.mil.fixture"))).readText())
            .getJSONArray("exchanges")
        val notices = mutableListOf<MilNotice>()
        val mil = LiveMil(notices::add)
        for ((i, command) in listOf(ElmCommand.MilStatus, ElmCommand.StoredDtc, ElmCommand.PendingDtc).withIndex()) {
            val row = fixture.getJSONObject(i)
            assertEquals(row.getString("command"), command.wire)
            if (i > 0) assertEquals(command, mil.nextDtc())
            mil.reply(command, frame(command, row.getString("raw")), i * 100L, base, i * 100L)
        }
        assertEquals("on", mil.snapshot(1000, true).getString("state"))
        assertEquals(0, mil.snapshot(1000, true).getInt("dtc_count"))
        assertEquals("MIL_ALREADY_ON", notices.first().kind)
        assertEquals(listOf("03", "07"), notices.drop(1).map { it.fields.getString("command") })
        for (notice in notices.drop(1)) {
            assertEquals("available", notice.fields.getString("status"))
            assertEquals(0, notice.fields.getJSONArray("codes").length())
            assertEquals(notices.first().fields.getString("incident_id"), notice.fields.getString("incident_id"))
        }
        assertNull(mil.nextDtc())
    }

    @Test fun freshOffToOnIsDistinctFromAfterGapAndNeverRepeatsOnContinuity() {
        val notices = mutableListOf<MilNotice>()
        val mil = LiveMil(notices::add)
        fun reply(tick: Long, raw: String) =
            mil.reply(ElmCommand.MilStatus, frame(ElmCommand.MilStatus, "$raw\r>"), tick, base.plusMillis(tick), tick)
        reply(0, "41010007E1A1")
        reply(5000, "41018207E1A1")
        reply(10000, "41018207E1A1")
        mil.expire(25000)
        assertEquals("unknown", mil.snapshot(25000, true).getString("state"))
        assertEquals(25000L, notices.last().observedTick)
        reply(26000, "41018207E1A1")
        reply(31000, "41010007E1A1")
        assertEquals(listOf("MIL_INITIAL_OFF", "MIL_ON", "MIL_UNKNOWN", "MIL_ON_AFTER_GAP", "MIL_OFF"),
            notices.filter { it.type == "mil_transition" }.map { it.kind })
        assertEquals("unknown", mil.snapshot(32000, false).getString("state"))
        mil.invalidate("generation_changed")
        assertNull(mil.nextDtc())
        assertTrue(notices.any { it.type == "mil_dtc" && it.fields.getString("status") == "interrupted" })
    }

    @Test fun explicitUnsupportedCancelsAlreadyQueuedReadsOfTheSameMode() {
        val notices = mutableListOf<MilNotice>()
        val mil = LiveMil(notices::add)
        fun state(tick: Long, raw: String) =
            mil.reply(ElmCommand.MilStatus, listOf(raw), tick, base.plusMillis(tick), tick)
        state(0, "41018007E1A1")
        state(5000, "41010007E1A1")
        state(10000, "41018007E1A1")
        mil.reply(ElmCommand.StoredDtc, listOf("7F0311"), 11000, base, 11000)
        mil.reply(ElmCommand.PendingDtc, listOf("470000"), 12000, base, 12000)
        assertEquals(ElmCommand.PendingDtc, mil.nextDtc())
        val records = notices.filter { it.type == "mil_dtc" && it.fields.getString("command") == "03" }
        assertEquals(2, records.size)
        assertEquals("previous_reply", records.last().fields.getString("reason"))
    }

    @Test fun incompleteOrConflictingMilAndOldReceiptsCannotBecomeOff() {
        val mil = LiveMil()
        fun reply(raw: String, receipt: Long, observed: Long) =
            mil.reply(ElmCommand.MilStatus, frame(ElmCommand.MilStatus, "$raw\r>"), receipt, base, observed)
        reply("41018007E1A1", 1000, 1000)
        reply("41010007E1A1", 900, 1200)
        assertEquals("on", mil.snapshot(1300, true).getString("state"))
        reply("41018007E1A1\r41010007E1A1", 2000, 2100)
        assertEquals("unknown", mil.snapshot(2100, true).getString("state"))
        reply("41010007E1", 2500, 2600)
        reply("41010007E1A1", 3000, 18000)
        assertEquals("unknown", mil.snapshot(18000, true).getString("state"))
    }

    @Test fun detailsIncludeOnlyRepliesForTheCurrentOnEpisode() {
        val mil = LiveMil()
        fun lamp(tick: Long, on: Boolean) = mil.reply(ElmCommand.MilStatus,
            listOf(if (on) "41018107E1A1" else "41010007E1A1"), tick, base.plusMillis(tick), tick)
        fun stored(tick: Long, raw: String) =
            mil.reply(ElmCommand.StoredDtc, listOf(raw), tick, base.plusMillis(tick), tick)
        fun pending(tick: Long) =
            mil.reply(ElmCommand.PendingDtc, listOf("470000"), tick, base.plusMillis(tick), tick)

        lamp(0, true)
        stored(100, "430420")
        val first = mil.snapshot(200, true).getJSONObject("dtc_replies")
        assertEquals("P0420", first.getJSONObject("03").getJSONArray("codes").getString(0))
        assertEquals(base.plusMillis(100).toString(), first.getJSONObject("03").getString("captured_at"))
        assertFalse(first.has("07"))
        assertFalse(mil.snapshot(200, true, includeDtc = false).has("dtc_replies"))

        lamp(300, false)
        assertFalse(mil.snapshot(300, true).has("dtc_replies"))
        lamp(400, true)
        pending(500) // Completion belongs to the previous ON episode.
        assertEquals(0, mil.snapshot(500, true).getJSONObject("dtc_replies").length())
        stored(600, "NO DATA")
        pending(700)
        val second = mil.snapshot(750, true).getJSONObject("dtc_replies")
        assertEquals("temporarily_unavailable", second.getJSONObject("03").getString("status"))
        assertEquals(0, second.getJSONObject("07").getJSONArray("codes").length())
        assertFalse(mil.snapshot(800, false).has("dtc_replies"))
        assertFalse(mil.snapshot(15_400, true).has("dtc_replies"))
        mil.invalidate("generation_changed")
        lamp(16_000, true)
        assertEquals(0, mil.snapshot(16_000, true).getJSONObject("dtc_replies").length())
    }

    @Test fun dtcRejectsUnexpectedModesAndPaddingAndPreservesDistinctStatuses() {
        val command = ElmCommand.StoredDtc
        fun decode(raw: String) = decodeDtc(command, frame(command, "$raw\r>"))
        assertEquals(listOf("P0420", "U0100"), decode("430420C1000000\r4304200000").codes)
        assertEquals("invalid_response", decode("4300000420").status)
        assertEquals("invalid_response", decode("470000").status)
        assertEquals("invalid_response", decode("43").status)
        assertEquals("temporarily_unavailable", decode("NO DATA").status)
        assertEquals("unsupported", decode("7F0311").status)
        assertEquals("temporarily_unavailable", decode("7F0331").status)
        assertNull(decode("7F0711").codes)
        assertEquals("invalid_response", decode("7F0711").status)
    }

    private fun frame(command: ElmCommand, raw: String) = ElmFrame().let {
        assertTrue(it.append(raw.toByteArray(Charsets.US_ASCII)))
        assertTrue(command.wire.isNotEmpty() && it.complete)
        it.lines(command)
    }
}
