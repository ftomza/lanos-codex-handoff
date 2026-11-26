package io.github.ftomza.lanos

import java.io.IOException
import java.io.OutputStream
import java.time.Instant
import java.util.UUID
import java.util.concurrent.Executor
import java.util.concurrent.Executors
import java.util.concurrent.Semaphore
import org.json.JSONObject

internal class TripJournal(
    private val archive: TripArchive,
    private val worker: Executor,
    private val onFailure: (Exception) -> Unit,
) {
    private data class MilWindow(val id: String, val start: Long, val deadline: Long)

    private var active: Session? = null
    private val slots = Semaphore(64)
    private val sessionSlots = Semaphore(8)
    val recording get() = active != null

    fun begin(tick: Long, at: Instant, mode: FuelMode, generation: Long) {
        if (active != null || mode == FuelMode.Simulator) return
        if (!sessionSlots.tryAcquire()) {
            onFailure(IOException("Trip journal worker busy"))
            return
        }
        val session = Session(tick, at, mode, generation)
        active = session
        run(session) { session.writer.open(tick, at) }
    }

    fun transition(tick: Long, at: Instant, stage: ProbeStage, generation: Long, recoveryRequired: Boolean) {
        event(tick, at, "connection_stage", JSONObject().put("stage", stage.name)
            .put("generation", generation).put("recovery_required", recoveryRequired))
    }

    fun generation(tick: Long, at: Instant, generation: Long) {
        val session = active ?: return
        if (generation == session.generation) return
        event(tick, at, "generation_changed", JSONObject().put("generation", generation))
        session.generation = generation
    }

    fun observe(tick: Long, at: Instant, generation: Long, covered: Boolean?, sample: () -> JSONObject) {
        val session = active ?: return
        generation(tick, at, generation)
        if (covered != null && covered != session.covered) {
            if (!covered || session.covered == false) {
                event(tick, at, if (covered) "coverage_resumed" else "coverage_gap_started")
            }
            session.covered = covered
        }
        if (tick - session.sampleTick < 1000) return
        session.sampleTick = tick
        submit(session, tick, at) {
            val reading = sample().put("type", "sample")
            session.samples.addLast(tick)
            if (session.samples.size > 96) session.samples.removeFirst()
            buildList {
                add(reading)
                while (session.milWindows.firstOrNull()?.let { tick >= it.deadline } == true) {
                    add(finishMilWindow(session, session.milWindows.removeFirst(), tick, "window_elapsed"))
                }
            }
        }
    }

    fun milNotice(tick: Long, at: Instant, generation: Long, notice: MilNotice) {
        val session = active ?: return
        val fields = notice.fields.put("kind", notice.kind).put("generation", generation)
        if (notice.type != "mil_transition" || !fields.has("incident_id")) {
            event(tick, at, notice.type, fields)
            return
        }
        submit(session, tick, at) {
            val from = tick - MIL_PRE_MS
            val horizon = tick + MIL_POST_MS
            val pre = session.samples.filter { it in from..tick }
            fields.put("context_from_elapsed_ms", (from - session.start).coerceAtLeast(0))
                .put("context_to_elapsed_ms", horizon - session.start)
                .put("pre_sample_count", pre.size)
                .put("pre_window_truncated", from < session.start || hasSampleGap(pre, from, tick))
            buildList {
                if (session.milWindows.size >= 8) {
                    add(finishMilWindow(session, session.milWindows.removeFirst(), tick, "window_limit"))
                }
                session.milWindows.addLast(MilWindow(fields.getString("incident_id"), tick, horizon))
                add(eventRow(notice.type, fields))
            }
        }
    }

    private fun finishMilWindow(session: Session, window: MilWindow, tick: Long, reason: String): JSONObject {
        val post = session.samples.filter { it > window.start && it <= window.deadline }
        val recorded = (post.lastOrNull() ?: window.start) - window.start
        return eventRow("mil_context_finished", JSONObject()
            .put("incident_id", window.id).put("reason", reason)
            .put("post_sample_count", post.size).put("post_ms_recorded", recorded)
            .put("post_window_truncated", tick < window.deadline || hasSampleGap(post, window.start, window.deadline)))
    }

    private fun hasSampleGap(samples: List<Long>, from: Long, to: Long): Boolean =
        samples.isEmpty() || samples.first() - from > MAX_SAMPLE_GAP_MS || to - samples.last() > MAX_SAMPLE_GAP_MS ||
            samples.zipWithNext().any { (a, b) -> b - a > MAX_SAMPLE_GAP_MS }

    fun event(tick: Long, at: Instant, name: String, fields: JSONObject = JSONObject()) {
        val session = active ?: return
        if (name == "fuel_configuration" || name == "fuel_reset") session.covered = null
        submit(session, tick, at) { listOf(eventRow(name, fields)) }
    }

    fun end(tick: Long, at: Instant, reason: String) {
        val session = active ?: return
        active = null
        val windows = session.milWindows.map { finishMilWindow(session, it, tick, reason) }
        val dropped = session.dropped
        run(session, finished = true) {
            session.writer.dropNotice(dropped, tick, at)
            // The existing reserved close operation also carries final context markers.
            for (row in windows) session.writer.record(row, tick, at)
            session.writer.finish(tick, at, reason)
        }
    }

    private fun submit(session: Session, tick: Long, at: Instant, rows: () -> List<JSONObject>) {
        if (!slots.tryAcquire()) {
            session.dropped++
            return
        }
        val dropped = session.dropped
        session.dropped = 0
        try {
            val records = rows()
            run(session, queued = true) {
                session.writer.dropNotice(dropped, tick, at)
                for (row in records) session.writer.record(row, tick, at)
            }
        } catch (e: Exception) {
            slots.release()
            throw e
        }
    }

    private fun eventRow(name: String, fields: JSONObject) = fields.put("type", "event").put("event", name)

    private fun run(session: Session, queued: Boolean = false, finished: Boolean = false, action: () -> Unit) {
        worker.execute {
            try {
                if (!session.failed) action()
            } catch (e: Exception) {
                session.failed = true
                try {
                    session.writer.close()
                } catch (close: IOException) {
                    e.addSuppressed(close)
                }
                onFailure(e)
            } finally {
                if (queued) slots.release()
                if (finished) sessionSlots.release()
            }
        }
    }

    private inner class Session(val start: Long, at: Instant, mode: FuelMode, var generation: Long) {
        val writer = Writer(start, at, mode)
        var sampleTick = start - 1000L
        var covered: Boolean? = null
        val milWindows = ArrayDeque<MilWindow>()
        val samples = ArrayDeque<Long>()
        var dropped = 0L
        var failed = false // Worker only.
    }

    private inner class Writer(private val started: Long, private val startedAt: Instant, private val mode: FuelMode) {
        private val id = UUID.randomUUID().toString()
        private var output: OutputStream? = null
        private var part = 0
        private var bytes = 0
        private var lastFlush = started

        fun open(tick: Long, at: Instant) {
            output = archive.openPart(startedAt)
            bytes = 0
            part++
            lastFlush = tick
            val header = JSONObject().put("type", "session_start").put("started_at", startedAt.toString())
                .put("initial_fuel_mode", mode.name)
            write(line(header, tick, at))
        }

        fun record(body: JSONObject, tick: Long, at: Instant) {
            var encoded = line(body, tick, at)
            if (bytes + encoded.size + endReserve > archive.maxPartBytes) {
                finish(tick, at, "rotation")
                open(tick, at)
                encoded = line(body, tick, at)
            }
            if (bytes + encoded.size + endReserve > archive.maxPartBytes) throw IOException("Journal row does not fit its part")
            write(encoded)
            if (tick - lastFlush >= 5000) {
                checkNotNull(output).flush()
                lastFlush = tick
            }
        }

        fun dropNotice(count: Long, tick: Long, at: Instant) {
            if (count == 0L) return
            record(JSONObject().put("type", "event").put("event", "records_dropped").put("count", count), tick, at)
        }

        fun finish(tick: Long, at: Instant, reason: String) {
            write(line(JSONObject().put("type", "session_end").put("reason", reason), tick, at))
            close()
        }

        fun close() {
            val stream = output
            output = null
            stream?.close()
        }

        private fun line(body: JSONObject, tick: Long, at: Instant): ByteArray {
            val payload = body.put("schema", "lanos.android.trip").put("version", 1)
                .put("session_id", id).put("part", part).put("source", "live")
                .put("at", at.toString()).put("elapsed_ms", maxOf(0, tick - started)).toString()
            return (payload + "\n").toByteArray(Charsets.UTF_8).also {
                if (it.size > TripArchive.maxLineBytes) throw IOException("Journal row too large")
            }
        }

        private fun write(encoded: ByteArray) {
            if (bytes + encoded.size > archive.maxPartBytes) throw IOException("Journal part too large")
            checkNotNull(output).write(encoded)
            bytes += encoded.size
        }
    }

    companion object {
        val io: Executor = Executors.newSingleThreadExecutor()
        private const val endReserve = 512
        private const val MIL_PRE_MS = 60_000L
        private const val MIL_POST_MS = 30_000L
        private const val MAX_SAMPLE_GAP_MS = 1750L
    }
}
