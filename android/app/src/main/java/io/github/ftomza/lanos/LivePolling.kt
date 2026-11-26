package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject

internal class LivePolling(
    val mil: LiveMil = LiveMil(),
    private val generationChanged: (Long) -> Unit = {},
) {
    private enum class Signal(val command: ElmCommand, val key: String, val unit: String, val ttl: Long, val instrument: Boolean = false) {
        Rpm(ElmCommand.Rpm, "rpm", "rpm", 3000, instrument = true),
        Map(ElmCommand.Map, "map_kpa", "kPa", 3000),
        Speed(ElmCommand.Speed, "speed_kmh", "km/h", 3000, instrument = true),
        Coolant(ElmCommand.Coolant, "coolant_c", "degC", 10_000, instrument = true),
        Voltage(ElmCommand.Voltage, "voltage_v", "V", 10_000, instrument = true),
        Iat(ElmCommand.Iat, "iat_c", "degC", 12_000),
        ShortTrim(ElmCommand.ShortTrim, "stft_b1_percent", "%", 12_000),
        LongTrim(ElmCommand.LongTrim, "ltft_b1_percent", "%", 35_000),
    }

    private data class Sample(val value: Double?, val status: String, val tick: Long, val at: Instant)
    private val samples = mutableMapOf<Signal, Sample>()
    private var bitmap: List<Int>? = null
    var generation = 0L
        private set

    fun reset() {
        mil.invalidate("generation_changed")
        samples.clear()
        bitmap = null
        generationChanged(++generation)
    }

    fun commands() = buildList {
        addAll(Signal.entries.filter { support(it.command.pid) == "supported" }.map { it.command })
        if (support(ElmCommand.MilStatus.pid) == "supported") add(ElmCommand.MilStatus)
    }

    fun hasBitmap(): Boolean = bitmap != null

    fun clearEcu() {
        mil.invalidate("ecu_standby")
        bitmap = null
        samples.keys.removeAll { it.command.pid != null }
        generationChanged(++generation)
    }

    fun succeeded(command: ElmCommand): Boolean = if (command == ElmCommand.MilStatus) mil.hasReading
        else samples.any { (signal, sample) -> signal.command == command && sample.value != null }

    fun reply(command: ElmCommand, frame: ElmFrame, tick: Long, at: Instant, observedTick: Long = tick): Boolean {
        val lines = frame.lines(command)
        when (command) {
            ElmCommand.EchoOff, ElmCommand.LinefeedsOff, ElmCommand.AutoProtocol ->
                return lines.singleOrNull()?.equals("OK", ignoreCase = true) == true
            ElmCommand.Identity -> return frame.identity() != null
            ElmCommand.Capabilities -> {
                bitmap = payload(lines, 0, 4)
                return true
            }
            ElmCommand.MilStatus, ElmCommand.StoredDtc, ElmCommand.PendingDtc -> {
                mil.reply(command, lines, tick, at, observedTick)
                return true
            }
            ElmCommand.Protocol -> return true // Informational; bitmap is the capability evidence.
            else -> Unit
        }

        val signal = Signal.entries.first { it.command == command }
        val value = when (command) {
            ElmCommand.Rpm -> payload(lines, 0x0c, 2)?.let { (256 * it[0] + it[1]) / 4.0 }
            ElmCommand.Speed, ElmCommand.Map -> singleByte(command, lines)?.toDouble()
            ElmCommand.Coolant, ElmCommand.Iat -> singleByte(command, lines)?.let { (it - 40).toDouble() }
            ElmCommand.ShortTrim, ElmCommand.LongTrim -> singleByte(command, lines)?.let { (it - 128) * 100.0 / 128 }
            ElmCommand.Voltage -> lines.singleOrNull()?.let { voltage.matchEntire(it) }
                ?.groupValues?.get(1)?.toDoubleOrNull()?.takeIf { it in 0.0..32.0 }
            else -> null
        }
        val status = when {
            value != null -> "available"
            lines.isEmpty() || lines.any { it.uppercase() in noData || it.uppercase().startsWith("7F") } ->
                "temporarily_unavailable"
            else -> "invalid_response"
        }
        samples[signal] = Sample(value, status, tick, at)
        return true
    }

    fun fuelInputs(stage: ProbeStage, tick: Long): FuelInputs {
        if (stage != ProbeStage.Live) return FuelInputs(false, generation)
        fun reading(signal: Signal): FuelReading? =
            samples[signal]?.takeIf { support(signal.command.pid) == "supported" && isFresh(signal, it, tick) }
                ?.let { FuelReading(checkNotNull(it.value), it.tick, signal.ttl) }
        return FuelInputs(true, generation, reading(Signal.Rpm), reading(Signal.Map), reading(Signal.Iat), reading(Signal.Speed))
    }

    fun json(result: ProbeResult, tick: Long, now: Instant): String = snapshot(result, tick, now).toString()

    fun snapshot(
        result: ProbeResult, tick: Long, now: Instant,
        includeSignalAge: Boolean = false, includeDtc: Boolean = true,
    ): JSONObject {
        val waitingForEcu = result.stage == ProbeStage.WaitingEcu
        fun visible(signal: Signal) = result.stage == ProbeStage.Live || (waitingForEcu && signal == Signal.Voltage)
        val available = samples.filter { (signal, sample) -> visible(signal) && isFresh(signal, sample, tick) }
        val milReading = mil.snapshot(tick, result.stage == ProbeStage.Live, includeDtc)
            .put("support", support(ElmCommand.MilStatus.pid))
        val latest = available.values.maxByOrNull { it.tick }?.at
            ?: milReading.optString("captured_at").takeUnless { milReading.isNull("captured_at") }?.let(Instant::parse)
        val remaining = available.maxOfOrNull { (signal, sample) -> signal.ttl - (tick - sample.tick) }
            ?: milReading.getLong("remaining_ms")
        val health = when {
            waitingForEcu -> if (latest != null) "connected" else "connecting"
            result.stage == ProbeStage.Live -> if (latest != null) "connected" else "stale"
            result.stage in waitingStages -> "connecting"
            else -> "disconnected"
        }
        val body = JSONObject().put("source", "live").put("health", health)
            .put("server_time", now.toString())
            .put("captured_at", latest?.toString() ?: JSONObject.NULL)
            .put("received_at", (latest ?: now).toString())
            .put("stale_after_ms", 10_000)
            .put("remaining_ms", remaining)
            .put("message", result.detail)
            .put("recovery_required", result.recoveryRequired)
        when (result.stage) {
            ProbeStage.WaitingEcu -> body.put("standby_reason", "ecu")
            ProbeStage.Retrying -> body.put("standby_reason", "adapter")
            ProbeStage.Recovering -> body.put("standby_reason", "synchronize")
            else -> Unit
        }

        val signals = JSONObject()
        for (signal in Signal.entries) {
            val sample = samples[signal]?.takeIf { visible(signal) }
            val supported = support(signal.command.pid)
            val value = available[signal]?.value
            val status = when {
                value != null -> "available"
                sample?.value != null -> "stale"
                sample != null -> sample.status
                supported == "unsupported" -> "unsupported"
                else -> "temporarily_unavailable"
            }
            if (signal.instrument) body.put(signal.key, value ?: JSONObject.NULL)
            val fields = JSONObject()
                .put("value", value ?: JSONObject.NULL)
                .put("unit", signal.unit)
                .put("source", if (signal.command.pid == null) "adapter_atrv" else "ecu_mode01")
                .put("status", status)
                .put("support", supported)
                .put("captured_at", sample?.at?.toString() ?: JSONObject.NULL)
                .put("received_at", sample?.at?.toString() ?: JSONObject.NULL)
                .put("remaining_ms", if (value != null && sample != null) signal.ttl - (tick - sample.tick) else 0L)
                .put("stale_after_ms", signal.ttl)
            if (includeSignalAge) fields.put("age_ms", sample?.let { (tick - it.tick).takeIf { age -> age >= 0 } } ?: JSONObject.NULL)
            signals.put(signal.key, fields)
        }
        return body.put("signals", signals).put("mil", milReading)
    }

    private fun isFresh(signal: Signal, sample: Sample, tick: Long) =
        sample.value != null && tick >= sample.tick && tick - sample.tick < signal.ttl

    private fun support(pid: Int?): String {
        if (pid == null) return "supported"
        val mask = bitmap ?: return "not_yet_tested"
        val bit = pid - 1
        return if (mask[bit / 8] and (0x80 shr (bit % 8)) != 0) "supported" else "unsupported"
    }

    private companion object {
        val waitingStages = setOf(
            ProbeStage.Scanning, ProbeStage.Connecting, ProbeStage.Discovering, ProbeStage.Subscribing,
            ProbeStage.Reading, ProbeStage.Recovering, ProbeStage.Retrying,
        )
        val noData = setOf("NO DATA", "STOPPED", "UNABLE TO CONNECT", "?")
        val voltage = Regex("([0-9]{1,2}(?:\\.[0-9]{1,3})?)\\s*V", RegexOption.IGNORE_CASE)

        fun singleByte(command: ElmCommand, lines: List<String>): Int? =
            payload(lines, checkNotNull(command.pid), 1)?.first()

        fun payload(lines: List<String>, pid: Int, bytes: Int): List<Int>? {
            if (lines.isEmpty()) return null
            val payloads = lines.map { line ->
                val hex = line.filterNot(Char::isWhitespace)
                if (hex.length != (bytes + 2) * 2 || !hex.all { it in '0'..'9' || it.uppercaseChar() in 'A'..'F' }) {
                    return null
                }
                val decoded = hex.chunked(2).map { it.toInt(16) }
                if (decoded[0] != 0x41 || decoded[1] != pid) return null
                decoded.drop(2)
            }
            return payloads.distinct().singleOrNull()
        }
    }
}
