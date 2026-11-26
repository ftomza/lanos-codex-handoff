package io.github.ftomza.lanos

import java.io.IOException
import java.time.Instant
import org.json.JSONArray
import org.json.JSONException
import org.json.JSONObject
import org.json.JSONTokener

internal data class TripPart(val name: String, val number: Int, val elapsed: Long)
internal data class TripSessionRef(val id: String, val startedAt: Instant, val parts: List<TripPart>)

internal class TripHistory(private val archive: TripArchive) {
    fun catalog(): List<TripSessionRef> {
        val sessions = linkedMapOf<String, TripSessionRef>()
        for (name in archive.entries().take(24)) {
            try {
                archive.visitPart(name) { line ->
                    val row = parseTripRow(line)
                    val start = row?.text("started_at")?.let(::parseInstant)
                    if (row?.text("type") != "session_start" || start == null) return@visitPart false
                    val id = row.getString("session_id")
                    val part = TripPart(name, row.getInt("part"), row.getLong("elapsed_ms"))
                    val prior = sessions[id]
                    sessions[id] = TripSessionRef(id, prior?.startedAt ?: start, prior?.parts.orEmpty() + part)
                    false
                }
            } catch (_: IOException) {
                // The part remains available from the JSONL export list.
            }
        }
        return sessions.values.map { it.copy(parts = it.parts.sortedBy(TripPart::number)) }
    }

    @Throws(IOException::class)
    fun report(ref: TripSessionRef): JSONObject {
        val builder = ReportBuilder(ref.startedAt)
        var previousPart = 0
        var previousReason: String? = null
        for (part in ref.parts) {
            if (Thread.currentThread().isInterrupted) throw IOException("Journal read interrupted")
            if (part.number != previousPart + 1 || (previousPart > 0 && previousReason != "rotation")) builder.breakData()
            if (part.number <= previousPart) {
                builder.badRow()
                continue
            }
            previousPart = part.number
            var first = true
            var footer: String? = null
            previousReason = null
            try {
                archive.visitPart(part.name) { line ->
                    if (builder.exhausted) return@visitPart false
                    val row = parseTripRow(line)
                    if (first) {
                        first = false
                        if (row?.text("type") != "session_start" || row.text("session_id") != ref.id ||
                            row.text("started_at")?.let(::parseInstant) != ref.startedAt ||
                            row.integer("part") != part.number.toLong() || row.integer("elapsed_ms") != part.elapsed ||
                            !builder.advance(part.elapsed)
                        ) {
                            builder.badRow()
                            return@visitPart false
                        }
                        return@visitPart true
                    }
                    if (footer != null || row == null || row.text("session_id") != ref.id ||
                        row.integer("part") != part.number.toLong() ||
                        !builder.advance(row.getLong("elapsed_ms"))
                    ) {
                        builder.badRow()
                        return@visitPart true
                    }
                    when (row.getString("type")) {
                        "sample" -> builder.sample(row)
                        "event" -> builder.event(row)
                        "session_end" -> footer = row.text("reason") ?: "unknown"
                        else -> builder.badRow()
                    }
                    true
                }
                previousReason = footer
            } catch (e: IOException) {
                if (Thread.currentThread().isInterrupted) throw e
                builder.badRow()
            }
        }
        val closed = previousReason != null && previousReason != "rotation"
        return builder.json(closed)
    }
}

private class ReportBuilder(private val start: Instant) {
    private val samples = ArrayList<HistorySample>()
    private val markers = ArrayList<TripMarker>()
    private var damaged = false
    private var forceBreak = true
    private var forceCounterBreak = false
    private var forceFlowBreak = false
    private var badRows = 0
    private var omittedMarkers = 0
    private var rows = 0
    private var tick = 0L
    private val totals = TripTotals()
    val exhausted get() = rows >= 50_000

    fun breakData() {
        damaged = true
        forceBreak = true
    }

    fun badRow() {
        badRows++
        rows++
        breakData()
    }

    fun advance(next: Long): Boolean {
        if (next < tick) return false
        tick = next
        rows++
        return true
    }

    fun sample(row: JSONObject) {
        val previous = samples.lastOrNull()
        val generation = row.integer("generation")
        val key = CounterValues.parse(row)
        val differentGeneration = previous != null && (generation == null || generation != previous.generation)
        val dt = previous?.let { tick - it.tick } ?: 0
        if (previous != null && dt <= 0) {
            badRow()
            return
        }
        val gap = forceBreak || differentGeneration || dt > 3000
        if (dt > 3000) breakData()
        val fuel = row.optJSONObject("fuel")
        val signals = row.optJSONObject("signals")
        val values = DoubleArray(tripCharts.size) { index ->
            val id = tripCharts[index]
            val value = when (id) {
                "flow_lph" -> fuel?.takeIf { it.text("status") == "available" }?.number(id, 0.0, 400.0)
                "liters", "average_l100" -> fuel?.number(id, 0.0, 1e9)
                else -> signals?.optJSONObject(id)?.takeIf { it.text("status") == "available" }
                    ?.number("value", -1e5, 1e5)
            }
            value ?: Double.NaN
        }
        val counterChanged = previous != null && (key == null || previous.counters?.key != key.key)
        val counterBreak = forceCounterBreak || counterChanged
        val sample = HistorySample(tick, generation, gap, counterBreak, forceFlowBreak, key, values)
        forceBreak = false
        forceCounterBreak = false
        forceFlowBreak = false
        if (previous != null && !gap && !counterBreak && previous.counters != null && key != null) {
            if (!totals.add(previous, sample, dt)) {
                damaged = true
                samples.add(sample.copy(breakCounters = true))
                return
            }
        }
        samples.add(sample)
    }

    fun event(row: JSONObject) {
        val name = row.text("event")
        if (name == "mil_dtc") {
            recordDtc(row)
            return
        }
        val label = when (name) {
            "coverage_gap_started", "records_dropped" -> "gap"
            "coverage_resumed" -> "coverage_resumed"
            "connection_stage" -> when (row.text("stage")) {
                "WaitingEcu" -> "waiting_ecu"
                "Retrying", "Failed", "Recovering" -> "connection_lost"
                "Live" -> "live_returned"
                else -> null
            }
            "mil_transition" -> when (row.text("kind")) {
                "MIL_ON" -> "mil_on"
                "MIL_ALREADY_ON" -> "mil_already_on"
                "MIL_ON_AFTER_GAP" -> "mil_after_gap"
                "MIL_OFF" -> "mil_off"
                "MIL_OFF_AFTER_GAP" -> "mil_off_after_gap"
                else -> null
            }
            "fuel_reset" -> "fuel_reset"
            "fuel_calibration", "fuel_configuration" -> "fuel_changed"
            "vgate_power_cycle_confirmed" -> "vgate_power_cycle"
            else -> null
        }
        if (name == "records_dropped") breakData()
        if (name == "generation_changed" || label == "waiting_ecu" || label == "connection_lost") forceBreak = true
        if (label == "gap" || label == "coverage_resumed") forceFlowBreak = true
        if (label == "fuel_reset" || label == "fuel_changed") forceCounterBreak = true
        if (label != null) {
            val incident = if (label in milOnMarkers) incidentKey(row)?.let(::TripIncident) else null
            addMarker(TripMarker(tick, label, name == "mil_transition", incident))
        }
    }

    private fun recordDtc(row: JSONObject) {
        val command = row.text("command")?.takeIf { it == "03" || it == "07" } ?: return
        val key = incidentKey(row) ?: return
        val incident = markers.lastOrNull { it.incident?.key == key }?.incident ?: TripIncident(key).also {
            addMarker(TripMarker(tick, "mil_dtc_result", mil = true, incident = it))
        }
        incident.replies.putIfAbsent(command, recordedDtc(row))
    }

    private fun addMarker(marker: TripMarker) {
        if (markers.size == 256) {
            omittedMarkers++
            if (!marker.mil) return
            val ordinary = markers.indexOfFirst { !it.mil }
            markers.removeAt(ordinary.coerceAtLeast(0))
        }
        markers.add(marker)
    }

    fun json(closed: Boolean): JSONObject {
        val duration = tick
        val series = JSONObject()
        for ((index, name) in tripCharts.withIndex()) {
            if (Thread.currentThread().isInterrupted) throw IOException("Journal read interrupted")
            series.put(name, chartPoints(samples, index, start, duration))
        }
        val events = JSONArray()
        for (marker in markers) {
            events.put(JSONObject().put("at", wall(start, marker.tick)).put("kind", marker.kind).apply {
                marker.incident?.let { put("dtc_replies", it.replies.dtcJson()) }
            })
        }
        return JSONObject().put("from", start.toString()).put("to", wall(start, tick)).put("duration_ms", duration)
            .put("closed", closed).put("incomplete", !closed || damaged || exhausted).put("bad_rows", badRows)
            .put("samples", samples.size).put("downsampled", samples.size > 1024)
            .put("series", series).put("markers", events).put("omitted_markers", omittedMarkers)
            .put("distance_km", if (totals.hasDistance) totals.km else JSONObject.NULL)
            .put("liters", if (totals.hasFuel) totals.liters else JSONObject.NULL)
            .put("average_l100", if (totals.jointKm >= 0.1) totals.jointLiters * 100 / totals.jointKm else JSONObject.NULL)
            .put("coverage_percent", if (duration > 0 && samples.isNotEmpty()) 100.0 * totals.coveredMs / duration else JSONObject.NULL)
    }
}

private data class TripMarker(val tick: Long, val kind: String, val mil: Boolean, val incident: TripIncident? = null)
private data class TripIncidentKey(val id: String, val generation: Long)
private data class TripIncident(val key: TripIncidentKey, val replies: MutableMap<String, MilDtcReading> = linkedMapOf())

private val milOnMarkers = setOf("mil_on", "mil_already_on", "mil_after_gap")
private val dtcCode = Regex("[PCBU][0-3][0-9A-F]{3}")
private val dtcFailureStatuses = setOf("temporarily_unavailable", "invalid_response", "unsupported", "interrupted")

private fun incidentKey(row: JSONObject): TripIncidentKey? {
    val id = row.text("incident_id")?.takeIf(sessionId::matches) ?: return null
    val generation = row.integer("generation")?.takeIf { it >= 0 } ?: return null
    return TripIncidentKey(id, generation)
}

private fun recordedDtc(row: JSONObject): MilDtcReading {
    val at = row.text("captured_at")?.let(::parseInstant)
    val status = row.text("status")
    if (status in dtcFailureStatuses) return MilDtcReading(checkNotNull(status), at)
    val invalid = MilDtcReading("invalid_response", at)
    if (status != "available" || at == null) return invalid
    val codes = row.optJSONArray("codes") ?: return invalid
    val truncated = if (row.has("codes_truncated")) row.opt("codes_truncated") as? Boolean ?: return invalid else false
    if (codes.length() > MAX_DTC_CODES || (truncated && codes.length() == 0)) return invalid
    val values = buildList {
        for (index in 0 until codes.length()) {
            val code = (codes.opt(index) as? String)?.takeIf(dtcCode::matches) ?: return invalid
            add(code)
        }
    }
    return MilDtcReading("available", at, values.distinct(), truncated)
}

internal data class HistorySample(
    val tick: Long, val generation: Long?, val breakAll: Boolean, val breakCounters: Boolean, val breakFlow: Boolean,
    val counters: CounterValues?, val values: DoubleArray,
)

internal data class CounterKey(val revision: Long, val resetAt: Instant, val factor: Double, val mode: String, val method: String?)
internal data class CounterValues(
    val key: CounterKey, val liters: Double, val km: Double, val coveredKm: Double,
    val jointLiters: Double?, val observedMs: Long, val coveredMs: Long,
) {
    companion object {
        fun parse(row: JSONObject): CounterValues? {
            val fuel = row.optJSONObject("fuel") ?: return null
            val coverage = row.optJSONObject("coverage") ?: return null
            val revision = fuel.integer("revision")?.takeIf { it > 0 } ?: return null
            val reset = coverage.text("reset_at")?.let(::parseInstant) ?: return null
            val factor = fuel.number("factor", 0.25, 4.0) ?: return null
            val mode = row.text("fuel_mode")?.takeIf { it == "Live" || it == "Experimental" } ?: return null
            val km = fuel.number("distance_km", 0.0, 1e9) ?: return null
            val liters = fuel.number("liters", 0.0, 1e9) ?: return null
            val covered = fuel.number("covered_km", 0.0, km + 1e-9) ?: return null
            val observed = coverage.integer("observed_ms")?.takeIf { it >= 0 } ?: return null
            val coveredTime = coverage.integer("covered_ms")?.takeIf { it in 0..observed } ?: return null
            val joint = coverage.number("joint_liters", 0.0, liters + 1e-9)
                ?: fuel.number("average_l100", 0.0, 1e9)?.let { it * covered / 100 }
                    ?.takeIf { it <= liters + 1e-9 }
                ?: when { coveredTime == 0L -> 0.0; coveredTime == observed -> liters; else -> null }
            return CounterValues(CounterKey(revision, reset, factor, mode, fuel.text("method")), liters, km, covered, joint, observed, coveredTime)
        }
    }
}

private class TripTotals {
    var km = 0.0
    var liters = 0.0
    var jointKm = 0.0
    var jointLiters = 0.0
    var coveredMs = 0L
    var hasFuel = false
    var hasDistance = false

    fun add(previous: HistorySample, current: HistorySample, elapsed: Long): Boolean {
        val a = checkNotNull(previous.counters)
        val b = checkNotNull(current.counters)
        val distance = b.km - a.km
        val fuel = b.liters - a.liters
        val covered = b.coveredKm - a.coveredKm
        val observed = b.observedMs - a.observedMs
        val jointMs = b.coveredMs - a.coveredMs
        if (distance < -1e-9 || fuel < -1e-9 || covered < -1e-9 || covered > distance + 1e-9 ||
            observed !in 0..elapsed || jointMs !in 0..observed) return false
        val joint = if (a.jointLiters != null && b.jointLiters != null) b.jointLiters - a.jointLiters
            else if (jointMs == observed) fuel else null
        if (joint != null && (joint < -1e-9 || joint > fuel + 1e-9)) return false
        km += distance.coerceAtLeast(0.0)
        liters += fuel.coerceAtLeast(0.0)
        coveredMs += jointMs
        if (joint != null) {
            jointLiters += joint.coerceAtLeast(0.0)
            jointKm += covered.coerceAtLeast(0.0)
        }
        hasDistance = hasDistance || distance > 0 ||
            (previous.values[speedChart].isFinite() && current.values[speedChart].isFinite())
        hasFuel = hasFuel || fuel > 0 ||
            (previous.values[flowChart].isFinite() && current.values[flowChart].isFinite())
        return true
    }
}

internal val tripCharts = listOf("rpm", "speed_kmh", "map_kpa", "iat_c", "flow_lph", "liters", "average_l100", "voltage_v")
private val speedChart = tripCharts.indexOf("speed_kmh")
private val flowChart = tripCharts.indexOf("flow_lph")

private val sessionId = Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}")
private val rowTypes = setOf("session_start", "session_end", "event", "sample")
internal fun parseTripRow(line: String?): JSONObject? {
    if (line == null || '\u0000' in line) return null
    return try {
        val input = JSONTokener(line)
        val row = input.nextValue() as? JSONObject ?: return null
        if (input.nextClean() != '\u0000' || row.text("schema") != "lanos.android.trip" ||
            row.integer("version") != 1L || row.text("source") != "live" ||
            row.text("session_id")?.matches(sessionId) != true ||
            row.integer("part")?.let { it in 1..100_000 } != true ||
            row.integer("elapsed_ms")?.let { it in 0..31_536_000_000L } != true ||
            row.text("type") !in rowTypes
        ) null else row
    } catch (_: JSONException) { null }
}

internal fun JSONObject.text(key: String) = opt(key) as? String
internal fun JSONObject.integer(key: String): Long? = when (val value = opt(key)) {
    is Int -> value.toLong()
    is Long -> value
    else -> null
}
private fun JSONObject.number(key: String, min: Double, max: Double): Double? =
    (opt(key) as? Number)?.toDouble()?.takeIf { it in min..max }
private val journalDates = Instant.parse("0001-01-01T00:00:00Z")..Instant.parse("9998-12-31T23:59:59Z")
private fun parseInstant(text: String): Instant? = runCatching { Instant.parse(text) }.getOrNull()?.takeIf { it in journalDates }
internal fun wall(start: Instant, elapsed: Long): String = start.plusMillis(elapsed).toString()
