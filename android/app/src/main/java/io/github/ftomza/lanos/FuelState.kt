package io.github.ftomza.lanos

import java.io.IOException
import java.time.Instant
import org.json.JSONArray
import org.json.JSONObject
import org.json.JSONTokener

internal data class FuelState(
    val mode: FuelMode,
    val resetAt: Instant,
    val savedAt: Instant = resetAt,
    val revision: Long = 1,
    val baseLiters: Double = 0.0,
    val jointLiters: Double = 0.0,
    val distanceKm: Double = 0.0,
    val coveredKm: Double = 0.0,
    val observedMs: Long = 0,
    val coveredMs: Long = 0,
    val gapMs: Long = 0,
    val reasons: Set<String> = emptySet(),
    val factor: Double = 1.0,
    val previousFactor: Double? = null,
) {
    val incomplete get() = reasons.isNotEmpty()

    fun validate(expected: FuelMode): FuelState {
        require(mode == expected && revision in 1..9_007_199_254_740_991L)
        require(listOf(baseLiters, jointLiters, distanceKm, coveredKm).all { it in 0.0..1e9 })
        require(jointLiters <= baseLiters + 1e-9 && coveredKm <= distanceKm + 1e-9)
        require(observedMs >= 0 && coveredMs in 0..observedMs && gapMs == observedMs - coveredMs)
        require(gapMs == 0L || incomplete)
        require(reasons.all(coverageReasons::contains))
        require(factor in factorRange && (previousFactor == null || previousFactor in factorRange))
        return this
    }

    fun json(): JSONObject = JSONObject()
        .put("version", VERSION).put("model", SpeedDensity.stateKey).put("mode", mode.name)
        .put("reset_at", resetAt.toString()).put("saved_at", savedAt.toString()).put("revision", revision)
        .put("base_liters", baseLiters).put("joint_liters", jointLiters)
        .put("distance_km", distanceKm).put("covered_km", coveredKm)
        .put("observed_ms", observedMs).put("covered_ms", coveredMs).put("gap_ms", gapMs)
        .put("reasons", JSONArray(reasons)).put("factor", factor)
        .put("previous_factor", previousFactor ?: JSONObject.NULL)

    companion object {
        private const val VERSION = 1
        private val coverageReasons = setOf(
            "process_restart", "background", "source_switch", "configuration_switch",
            "clock_reversal", "unavailable_input", "timing_or_generation_gap",
        )
        val factorRange = 0.25..4.0

        fun parse(data: String): FuelState {
            val reader = JSONTokener(data)
            val obj = reader.nextValue() as JSONObject
            while (reader.more()) require(reader.next() in " \n\t\r")
            require(obj.integer("version") == VERSION.toLong() && obj.get("model") == SpeedDensity.stateKey)
            val reasons = obj.getJSONArray("reasons")
            require(reasons.length() <= coverageReasons.size && obj.has("previous_factor"))
            return FuelState(
                mode = FuelMode.valueOf(obj.getString("mode")),
                resetAt = Instant.parse(obj.getString("reset_at")),
                savedAt = Instant.parse(obj.getString("saved_at")),
                revision = obj.integer("revision"),
                baseLiters = obj.number("base_liters"),
                jointLiters = obj.number("joint_liters"),
                distanceKm = obj.number("distance_km"),
                coveredKm = obj.number("covered_km"),
                observedMs = obj.integer("observed_ms"),
                coveredMs = obj.integer("covered_ms"),
                gapMs = obj.integer("gap_ms"),
                reasons = (0 until reasons.length()).map { reasons.get(it) as String }.toSet(),
                factor = obj.number("factor"),
                previousFactor = if (obj.isNull("previous_factor")) null else obj.number("previous_factor"),
            )
        }

        private fun JSONObject.integer(key: String): Long {
            val value = get(key)
            require(value is Int || value is Long)
            return (value as Number).toLong()
        }

        private fun JSONObject.number(key: String): Double = (get(key) as Number).toDouble()
    }
}

internal interface FuelStore {
    @Throws(IOException::class)
    fun load(): FuelState?

    @Throws(IOException::class)
    fun save(state: FuelState)
}
