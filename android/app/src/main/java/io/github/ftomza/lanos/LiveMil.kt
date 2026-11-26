package io.github.ftomza.lanos

import java.time.Instant
import java.util.Locale
import java.util.UUID
import org.json.JSONArray
import org.json.JSONObject

internal data class MilNotice(val type: String, val kind: String, val fields: JSONObject, val observedTick: Long? = null)
internal data class DtcResult(val status: String, val codes: List<String>? = null, val nrc: Int? = null)
internal const val MAX_DTC_CODES = 64

internal data class MilDtcReading(
    val status: String,
    val at: Instant?,
    val codes: List<String>? = null,
    val truncated: Boolean = false,
) {
    fun json() = JSONObject().put("status", status).put("captured_at", at?.toString() ?: JSONObject.NULL)
        .put("codes", codes?.let(::JSONArray) ?: JSONObject.NULL).put("codes_truncated", truncated)
}

internal fun Map<String, MilDtcReading>.dtcJson() = JSONObject().also { json ->
    for ((command, reading) in this) json.put(command, reading.json())
}

internal fun decodeDtc(command: ElmCommand, lines: List<String>): DtcResult {
    val mode = when (command) {
        ElmCommand.StoredDtc -> 0x03
        ElmCommand.PendingDtc -> 0x07
        else -> error("not a DTC command")
    }
    val normalized = lines.map(::compactDiagnostic).distinct()
    if (normalized.singleOrNull() in setOf("NODATA", "STOPPED", "UNABLETOCONNECT")) {
        return DtcResult("temporarily_unavailable")
    }
    val frames = normalized.map { hexFrame(it) ?: return DtcResult("invalid_response") }
    val negative = frames.singleOrNull()?.takeIf { it.size == 3 && it[0] == 0x7f && it[1] == mode }
    if (negative != null) {
        val nrc = negative[2]
        return DtcResult(if (nrc in setOf(0x11, 0x12)) "unsupported" else "temporarily_unavailable", nrc = nrc)
    }
    if (frames.isEmpty()) return DtcResult("invalid_response")
    val codes = linkedSetOf<String>()
    for (bytes in frames) {
        if (bytes.size < 3 || bytes.size % 2 != 1 || bytes[0] != mode + 0x40) {
            return DtcResult("invalid_response")
        }
        var padding = false
        for (i in 1 until bytes.size step 2) {
            val (a, b) = bytes[i] to bytes[i + 1]
            if (a == 0 && b == 0) {
                padding = true
                continue
            }
            if (padding) return DtcResult("invalid_response")
            codes += String.format(Locale.ROOT, "%c%X%X%02X", "PCBU"[a shr 6], (a shr 4) and 3, a and 15, b)
        }
    }
    return DtcResult("available", codes.toList())
}

private fun compactDiagnostic(line: String) = line.filterNot(Char::isWhitespace).uppercase(Locale.ROOT)

private fun hexFrame(line: String): List<Int>? {
    if (line.isEmpty() || line.length % 2 != 0 || line.any { it !in '0'..'9' && it !in 'A'..'F' }) return null
    return line.chunked(2).map { it.toInt(16) }
}

internal class LiveMil(private val emit: (MilNotice) -> Unit = {}) {
    private data class Reading(val bits: Int, val tick: Long, val at: Instant) {
        val on get() = bits and 0x80 != 0
        val count get() = bits and 0x7f
    }
    private data class Request(val incident: String, val command: ElmCommand)
    private data class Incident(val id: String, val replies: MutableMap<String, MilDtcReading> = linkedMapOf())

    private var latest: Reading? = null
    private var activeIncident: Incident? = null
    private var lastTick: Long? = null
    private var seen = false
    private val pending = ArrayDeque<Request>()
    private val unsupported = mutableMapOf<ElmCommand, Int>()

    fun newSession() {
        invalidate("session_restarted")
        seen = false
    }

    fun invalidate(reason: String) {
        unknown(reason)
        lastTick = null
        unsupported.clear()
        while (pending.isNotEmpty()) {
            sendResult(pending.removeFirst(), DtcResult("interrupted"), reason = reason)
        }
    }

    fun reply(command: ElmCommand, lines: List<String>, tick: Long, at: Instant, observedTick: Long) {
        if (command != ElmCommand.MilStatus) {
            val request = pending.firstOrNull()?.takeIf { it.command == command } ?: return
            pending.removeFirst()
            val result = decodeDtc(command, lines)
            sendResult(request, result, at)
            if (result.status == "unsupported") {
                unsupported[command] = checkNotNull(result.nrc)
                val skipped = pending.filter { it.command == command }
                pending.removeAll(skipped.toSet())
                for (item in skipped) sendResult(item, result, reason = "previous_reply")
            }
            return
        }
        expire(observedTick)
        if (lastTick?.let { tick <= it } == true) return
        if (tick > observedTick || observedTick - tick >= MIL_TTL_MS) return
        lastTick = tick
        val bytes = lines.map(::compactDiagnostic).distinct().singleOrNull()?.let(::hexFrame)
        if (bytes == null || bytes.size != 6 || bytes[0] != 0x41 || bytes[1] != 0x01) {
            unknown("mil_reply_unavailable")
            return
        }
        val reading = Reading(bytes[2], tick, at)
        val previous = latest
        latest = reading
        if (previous?.on == reading.on) return
        val kind = when {
            !seen && reading.on -> "MIL_ALREADY_ON"
            !seen -> "MIL_INITIAL_OFF"
            previous == null && reading.on -> "MIL_ON_AFTER_GAP"
            previous == null -> "MIL_OFF_AFTER_GAP"
            reading.on -> "MIL_ON"
            else -> "MIL_OFF"
        }
        seen = true
        val fields = JSONObject().put("state", if (reading.on) "on" else "off")
            .put("dtc_count", reading.count).put("captured_at", at.toString())
            .put("previous_captured_at", previous?.at?.toString() ?: JSONObject.NULL)
        val incident = if (reading.on) UUID.randomUUID().toString() else null
        activeIncident = incident?.let(::Incident)
        if (incident != null) fields.put("incident_id", incident)
        emit(MilNotice("mil_transition", kind, fields))
        if (incident != null) {
            enqueue(incident, ElmCommand.StoredDtc)
            enqueue(incident, ElmCommand.PendingDtc)
        }
    }

    private fun enqueue(incident: String, command: ElmCommand) {
        val request = Request(incident, command)
        val nrc = unsupported[command]
        when {
            nrc != null -> sendResult(request, DtcResult("unsupported", nrc = nrc), reason = "previous_reply")
            pending.size >= 8 -> sendResult(request, DtcResult("interrupted"), reason = "queue_limit")
            else -> pending.addLast(request)
        }
    }

    val hasReading get() = latest != null

    fun nextDtc(): ElmCommand? = pending.firstOrNull()?.command

    fun expire(tick: Long) {
        if (latest?.let { tick - it.tick >= MIL_TTL_MS } == true) unknown("expired", tick)
    }

    fun snapshot(tick: Long, connected: Boolean, includeDtc: Boolean = true): JSONObject {
        val reading = latest?.takeIf { connected && tick - it.tick in 0L until MIL_TTL_MS }
        return JSONObject().put("state", when (reading?.on) { true -> "on"; false -> "off"; null -> "unknown" })
            .put("dtc_count", reading?.count ?: JSONObject.NULL)
            .put("captured_at", reading?.at?.toString() ?: JSONObject.NULL)
            .put("remaining_ms", reading?.let { MIL_TTL_MS - (tick - it.tick) } ?: 0L)
            .apply {
                if (includeDtc && reading?.on == true) activeIncident?.let { put("dtc_replies", it.replies.dtcJson()) }
            }
    }

    private fun unknown(reason: String, tick: Long? = null) {
        activeIncident = null
        val previous = latest ?: return
        latest = null
        emit(MilNotice("mil_transition", "MIL_UNKNOWN", JSONObject().put("state", "unknown")
            .put("reason", reason).put("previous_captured_at", previous.at.toString()), observedTick = tick))
    }

    private fun sendResult(request: Request, result: DtcResult, at: Instant? = null, reason: String? = null) {
        val codes = result.codes
        val reading = MilDtcReading(result.status, at, codes?.take(MAX_DTC_CODES), (codes?.size ?: 0) > MAX_DTC_CODES)
        activeIncident?.takeIf { it.id == request.incident }?.replies?.put(request.command.wire, reading)
        emit(MilNotice("mil_dtc", "DTC_RESULT", reading.json()
            .put("incident_id", request.incident).put("command", request.command.wire)
            .put("negative_response_code", result.nrc ?: JSONObject.NULL)
            .put("reason", reason ?: JSONObject.NULL)))
    }

    companion object {
        private const val MIL_TTL_MS = 15_000L
    }
}
