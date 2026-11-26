package io.github.ftomza.lanos

import java.io.ByteArrayOutputStream
import java.time.Instant
import java.util.concurrent.Executor
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class TripJournalTest {
    @get:Rule val folder = TemporaryFolder()
    private val at = Instant.parse("2026-11-24T06:00:00Z")

    @Test fun recordsLiveTransitionsAndCoverageEvenBetweenSamples() {
        val archive = TripArchive(folder.root)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(100, at, FuelMode.Experimental, 1)
        var samples = 0
        fun observe(tick: Long, generation: Long, covered: Boolean?) =
            journal.observe(tick, at, generation, covered) { JSONObject().put("ordinal", ++samples) }
        observe(100, 1, null)
        journal.transition(150, at.minusSeconds(3), ProbeStage.WaitingEcu, 2, false)
        observe(250, 2, false)
        observe(300, 2, false)
        journal.transition(400, at, ProbeStage.Live, 3, false)
        observe(500, 3, true)
        observe(1100, 3, true)
        journal.end(1200, at, "screen_off")
        observe(2000, 3, true)

        val rows = read(archive, archive.entries().single())
        assertEquals(2, samples)
        assertEquals(listOf("session_start", "sample", "connection_stage", "generation_changed", "coverage_gap_started",
            "connection_stage", "generation_changed", "coverage_resumed", "sample", "session_end"),
            rows.map { it.optString("event", it.getString("type")) })
        assertEquals(listOf(0L, 0L, 50L, 150L, 150L, 300L, 400L, 400L, 1000L, 1100L),
            rows.map { it.getLong("elapsed_ms") })
        assertEquals("screen_off", rows.last().getString("reason"))
    }

    @Test fun milContextReferencesSamplesAndFlagsAnInterruptedTail() {
        val archive = TripArchive(folder.root)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 1)
        fun sample(tick: Long) = journal.observe(tick, at, 1, null) { JSONObject() }
        sample(0)
        sample(1000)
        journal.milNotice(1500, at, 1, MilNotice("mil_transition", "MIL_ON",
            JSONObject().put("incident_id", "first")))
        sample(2000)
        journal.milNotice(2300, at, 1, MilNotice("mil_dtc", "DTC_RESULT",
            JSONObject().put("incident_id", "first").put("command", "03").put("codes", org.json.JSONArray(listOf("P0420")))))
        sample(3000)
        journal.end(3200, at, "screen_off")
        val events = read(archive, archive.entries().single())
        val start = events.single { it.optString("event") == "mil_transition" }
        val end = events.single { it.optString("event") == "mil_context_finished" }
        assertEquals("first", end.getString("incident_id"))
        assertEquals(0L, start.getLong("context_from_elapsed_ms"))
        assertEquals(31_500L, start.getLong("context_to_elapsed_ms"))
        assertTrue(start.getBoolean("pre_window_truncated"))
        assertEquals(1500L, end.getLong("post_ms_recorded"))
        assertTrue(end.getBoolean("post_window_truncated"))
        assertEquals("screen_off", end.getString("reason"))
        assertEquals("P0420", events.single { it.optString("event") == "mil_dtc" }.getJSONArray("codes").getString(0))
    }

    @Test fun aSampleAfterTheWindowDoesNotHideARecordingGap() {
        val archive = TripArchive(folder.root)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 1)
        fun sample(tick: Long) = journal.observe(tick, at, 1, null) { JSONObject() }
        sample(0)
        journal.milNotice(60_000, at, 1, MilNotice("mil_transition", "MIL_ON",
            JSONObject().put("incident_id", "one")))
        sample(61_000)
        sample(95_000)
        journal.end(95_100, at, "background")
        val events = read(archive, archive.entries().single())
        val start = events.single { it.optString("event") == "mil_transition" }
        val end = events.single { it.optString("event") == "mil_context_finished" }
        assertTrue(start.getBoolean("pre_window_truncated"))
        assertEquals(1000L, end.getLong("post_ms_recorded"))
        assertTrue(end.getBoolean("post_window_truncated"))
    }

    @Test fun cleanCloseKeepsMilContextMarkerEvenWithAFullWorkerQueue() {
        val archive = TripArchive(folder.root)
        val queued = ArrayDeque<Runnable>()
        val journal = TripJournal(archive, Executor(queued::addLast)) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 0)
        journal.milNotice(0, at, 0, MilNotice("mil_transition", "MIL_ON",
            JSONObject().put("incident_id", "one")))
        repeat(64) { tick -> journal.event(tick.toLong(), at, "connection_stage") }
        journal.end(100, at, "background")
        while (queued.isNotEmpty()) queued.removeFirst().run()
        val events = read(archive, archive.entries().single())
        assertEquals("mil_context_finished", events[events.size - 2].getString("event"))
        assertEquals("one", events[events.size - 2].getString("incident_id"))
    }

    @Test fun recordsFastGenerationChangesBeforeTheNextSample() {
        val archive = TripArchive(folder.root)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(100, at, FuelMode.Live, 5)
        val polling = LivePolling { generation -> journal.generation(110, at, 5 + generation) }
        polling.reset()
        polling.clearEcu()
        journal.end(125, at, "background")
        val events = read(archive, archive.entries().single()).filter { it.optString("event") == "generation_changed" }
        assertEquals(listOf(6L, 7L), events.map { it.getLong("generation") })
    }

    @Test fun sessionsCannotBuildAnUnboundedQueueWhenStorageStalls() {
        val archive = TripArchive(folder.root)
        val queued = ArrayDeque<Runnable>()
        var failures = 0
        val journal = TripJournal(archive, Executor(queued::addLast)) { failures++ }
        repeat(100) { n ->
            journal.begin(n.toLong(), at, FuelMode.Live, 0)
            journal.end(n.toLong(), at, "background")
        }
        assertEquals(16, queued.size)
        assertEquals(92, failures)
        while (queued.isNotEmpty()) queued.removeFirst().run()
        journal.begin(200, at, FuelMode.Live, 0)
        journal.end(201, at, "background")
        while (queued.isNotEmpty()) queued.removeFirst().run()
        assertEquals(9, archive.entries().size)
    }

    @Test fun rotationKeepsBoundedPartsWithSharedSessionIdentity() {
        val archive = TripArchive(folder.root, maxPartBytes = 3000, maxParts = 2)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 0)
        repeat(12) { n ->
            journal.observe(n * 1000L, at, 0, null) { JSONObject().put("payload", "a".repeat(800)) }
        }
        journal.end(12_000, at, "background")
        val files = archive.entries()
        assertEquals(2, files.size)
        assertTrue(files.all { folder.root.resolve(it).length() <= 3000 })
        val newest = read(archive, files[0])
        val prior = read(archive, files[1])
        assertTrue(prior.first().getInt("part") > 1)
        assertEquals(prior.first().getInt("part") + 1, newest.first().getInt("part"))
        assertEquals(prior.first().getString("session_id"), newest.first().getString("session_id"))
        assertEquals("rotation", prior.last().getString("reason"))
        assertEquals("background", newest.last().getString("reason"))
    }

    @Test fun exportKeepsCompleteRowsAfterInterruptAndRejectsOtherPaths() {
        val archive = TripArchive(folder.root)
        val journal = TripJournal(archive, Executor { it.run() }) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 0)
        journal.observe(0, at, 0, null) { JSONObject().put("value", 7) }
        journal.end(1, at, "background")
        val name = archive.entries().single()
        val file = folder.root.resolve(name)
        val data = file.readText().trimEnd().substringBeforeLast('\n') + "\n"
        file.writeText(data + "{\"partial\":")
        folder.root.resolve("other.jsonl").writeText("other\n")
        assertEquals(listOf(name), archive.entries())
        val out = ByteArrayOutputStream()
        archive.export(name, out)
        assertEquals(data, out.toString("UTF-8"))
        assertFalse(read(archive, name).any { it.getString("type") == "session_end" })
        for (invalid in listOf("../$name", "other.jsonl", "/$name")) {
            assertTrue(runCatching { archive.export(invalid, ByteArrayOutputStream()) }.isFailure)
        }
    }

    @Test fun stalledWriterReportsDroppedRowsAndCanStillClose() {
        val archive = TripArchive(folder.root)
        val queued = ArrayDeque<Runnable>()
        val journal = TripJournal(archive, Executor(queued::addLast)) { throw AssertionError(it) }
        journal.begin(0, at, FuelMode.Live, 0)
        repeat(200) { n ->
            journal.observe(n * 1000L, at, 0, null) { JSONObject() }
        }
        journal.end(200_000, at, "background")
        assertTrue(queued.size <= 66)
        while (queued.isNotEmpty()) queued.removeFirst().run()
        val rows = read(archive, archive.entries().single())
        assertEquals(136L, rows.single { it.optString("event") == "records_dropped" }.getLong("count"))
        assertEquals("session_end", rows.last().getString("type"))
    }

    @Test fun storageFailureIsReportedOnceAndANewSessionCanRetry() {
        val dir = folder.root.resolve("journal")
        dir.writeText("file occupying journal path")
        val archive = TripArchive(dir)
        val failures = mutableListOf<Exception>()
        val journal = TripJournal(archive, Executor { it.run() }, failures::add)
        journal.begin(0, at, FuelMode.Live, 0)
        journal.observe(0, at, 0, null) { JSONObject() }
        journal.end(1, at, "background")
        assertEquals(1, failures.size)
        assertTrue(dir.delete())
        journal.begin(2, at, FuelMode.Live, 0)
        journal.end(3, at, "source_switch")
        assertEquals("source_switch", read(archive, archive.entries().single()).last().getString("reason"))
    }

    private fun read(archive: TripArchive, name: String): List<JSONObject> {
        val out = ByteArrayOutputStream()
        archive.export(name, out)
        return out.toString("UTF-8").lineSequence().filter(String::isNotEmpty).map(::JSONObject).toList()
    }
}
