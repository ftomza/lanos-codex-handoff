package io.github.ftomza.lanos

import java.io.IOException
import org.json.JSONObject

internal data class TripCleanupResult(val removed: Int, val skipped: Int)

// Use on TripJournal.io after the dashboard has closed its recording.
internal class TripCleanup(private val archive: TripArchive) {
    private val history = TripHistory(archive)

    fun emptySessions(): List<TripSessionRef> = history.catalog().filter(::isEmpty)

    fun remove(candidates: List<TripSessionRef>): TripCleanupResult {
        val current = history.catalog().associateBy(TripSessionRef::id)
        val removed = candidates.count { ref ->
            current[ref.id] == ref && isEmpty(ref) && deleteParts(ref)
        }
        return TripCleanupResult(removed, candidates.size - removed)
    }

    private fun deleteParts(ref: TripSessionRef): Boolean = try {
        ref.parts.asReversed().all { archive.deletePart(it.name) }
    } catch (_: IOException) {
        false
    }

    private fun isEmpty(ref: TripSessionRef): Boolean {
        var tick = 0L
        var counters: List<Double?>? = null
        for ((index, part) in ref.parts.withIndex()) {
            if (part.number != index + 1 || part.elapsed != tick) return false
            var header = true
            var footer: String? = null
            var valid = true
            try {
                archive.visitPart(part.name) { line ->
                    val row = parseTripRow(line)
                    val elapsed = row?.integer("elapsed_ms")
                    if (row == null || elapsed == null || elapsed < tick || footer != null ||
                        row.text("session_id") != ref.id || row.integer("part") != part.number.toLong()
                    ) {
                        valid = false
                        return@visitPart false
                    }
                    tick = elapsed
                    valid = if (header) {
                        header = false
                        row.text("type") == "session_start" && tick == part.elapsed &&
                            row.text("started_at") == ref.startedAt.toString()
                    } else when (row.text("type")) {
                        "sample" -> {
                            val current = emptySampleCounters(row)
                            val unchanged = counters == null || current == counters
                            counters = current
                            current != null && unchanged
                        }
                        "event" -> startupEvent(row)
                        "session_end" -> {
                            footer = row.text("reason")
                            !footer.isNullOrBlank()
                        }
                        else -> false
                    }
                    valid
                }
            } catch (_: IOException) {
                return false
            }
            val hasNext = index < ref.parts.lastIndex
            if (!valid || header || hasNext != (footer == "rotation")) return false
        }
        return ref.parts.isNotEmpty()
    }
}

private val startupStages = setOf("Idle", "Scanning", "Retrying", "Failed", "Cancelled")
private val unreadSignalStates = setOf("temporarily_unavailable", "unsupported")

private fun startupEvent(row: JSONObject): Boolean = when (row.text("event")) {
    "connection_stage" -> startupStage(row)
    "generation_changed", "coverage_gap_started" -> true
    else -> false
}

private fun startupStage(row: JSONObject) =
    row.text("stage") in startupStages && (!row.has("recovery_required") || row.opt("recovery_required") == false)

private fun emptySampleCounters(row: JSONObject): List<Double?>? {
    if (!startupStage(row)) return null
    val signals = row.optJSONObject("signals") ?: return null
    if (signals.length() == 0 || !signals.keys().asSequence().all { key ->
            val signal = signals.optJSONObject(key)
            signal != null && signal.text("status") in unreadSignalStates &&
                signal.opt("value") === JSONObject.NULL && signal.isNull("captured_at")
        }
    ) return null
    if (row.has("mil")) {
        val mil = row.optJSONObject("mil") ?: return null
        if (mil.text("state") != "unknown" || !mil.isNull("dtc_count") || !mil.isNull("captured_at")) return null
    }
    val fuel = row.optJSONObject("fuel") ?: return null
    if (fuel.text("status") != "unavailable" || fuel.opt("flow_lph") !== JSONObject.NULL || !fuel.isNull("speed_kmh")) return null
    val coverage = row.optJSONObject("coverage")
    if (coverage == null && !row.isNull("coverage")) return null
    // These totals may predate this session. Only changes are evidence of fuel accounting.
    return listOf(fuel.opt("liters"), fuel.opt("distance_km"), fuel.opt("covered_km"), coverage?.opt("joint_liters"))
        .map { value ->
            when (value) {
                null, JSONObject.NULL -> null
                is Number -> value.toDouble().takeIf { it.isFinite() && it >= 0 } ?: return null
                else -> return null
            }
        }
}
