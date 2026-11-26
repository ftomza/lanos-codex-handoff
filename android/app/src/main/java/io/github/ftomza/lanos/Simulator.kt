package io.github.ftomza.lanos

import java.time.Instant
import kotlin.math.exp
import kotlin.math.sin
import org.json.JSONObject

internal class Simulator {
    fun sample(elapsedMs: Long, now: Instant): Snapshot {
        val elapsed = elapsedMs.coerceAtLeast(0) / 1000.0
        val phase = elapsed % 60
        if (phase in 48.0..<54.0) {
            return Snapshot(now, null, "disconnected", "simulated connection loss")
        }
        if (phase in 42.0..<48.0) {
            val lastCapture = now.minusMillis(elapsedMs % 60_000 - 38_000)
            return Snapshot(now, lastCapture, "stale", "simulated missing updates")
        }
        val speed = when (phase) {
            in 5.0..<25.0 -> (phase - 5) * 4
            in 25.0..<35.0 -> 80.0
            in 35.0..<42.0 -> (45 - phase) * 8
            else -> 0.0
        }
        return Snapshot(
            now = now,
            capturedAt = now,
            health = "connected",
            message = "simulated driving cycle",
            rpm = 850 + speed * 35 + 20 * sin(phase * 2),
            speedKmh = speed,
            coolantC = 78 + 12 * (1 - exp(-elapsed / 90)),
            voltageV = if (phase in 24.0..<28.0) null else 14.1 + 0.12 * sin(phase / 3),
        )
    }
}

internal data class Snapshot(
    val now: Instant,
    val capturedAt: Instant?,
    val health: String,
    val message: String,
    val rpm: Double? = null,
    val speedKmh: Double? = null,
    val coolantC: Double? = null,
    val voltageV: Double? = null,
) {
    fun fuelInputs(tick: Long, generation: Long): FuelInputs {
        fun timed(value: Double?) = value?.let { FuelReading(it, tick, 3000) }
        if (health != "connected") return FuelInputs(false, generation)
        return FuelInputs(true, generation, timed(rpm), timed(speedKmh?.let { 32 + it * 0.55 }),
            timed(20.0), timed(speedKmh))
    }

    fun json(): String = payload().toString()

    fun payload(): JSONObject = JSONObject()
        .put("source", "simulator")
        .put("health", health)
        .put("message", message)
        .put("rpm", rpm ?: JSONObject.NULL)
        .put("speed_kmh", speedKmh ?: JSONObject.NULL)
        .put("coolant_c", coolantC ?: JSONObject.NULL)
        .put("voltage_v", voltageV ?: JSONObject.NULL)
        .put("captured_at", capturedAt?.toString() ?: JSONObject.NULL)
        .put("received_at", (capturedAt ?: now).toString())
        .put("server_time", now.toString())
        .put("stale_after_ms", 3000)
}
