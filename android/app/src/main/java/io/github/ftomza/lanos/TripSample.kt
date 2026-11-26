package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject

internal fun tripSample(polling: LivePolling, result: ProbeResult, tick: Long, at: Instant, computer: FuelComputer): JSONObject {
    val snapshot = polling.snapshot(result, tick, at, includeSignalAge = true, includeDtc = false)
    val signals = JSONObject()
    val available = snapshot.getJSONObject("signals")
    for (key in journalSignals) signals.put(key, available.getJSONObject(key))
    val fuel = computer.view(tick).apply { remove("persistence_error_label") }
    val state = computer.state
    val coverage = if (fuel.isNull("revision")) JSONObject.NULL else JSONObject()
        .put("observed_ms", state.observedMs).put("covered_ms", state.coveredMs).put("gap_ms", state.gapMs)
        .put("reset_at", state.resetAt.toString())
        .put("joint_liters", state.jointLiters * state.factor)
    return JSONObject().put("stage", result.stage.name).put("generation", polling.generation)
        .put("health", snapshot.getString("health")).put("recovery_required", result.recoveryRequired)
        .put("standby_reason", snapshot.optString("standby_reason").takeIf { it.isNotBlank() } ?: JSONObject.NULL)
        .put("fuel_mode", state.mode.name).put("signals", signals).put("fuel", fuel).put("coverage", coverage)
        .put("mil", snapshot.getJSONObject("mil"))
}

private val journalSignals = listOf("rpm", "map_kpa", "iat_c", "speed_kmh", "coolant_c", "voltage_v", "stft_b1_percent", "ltft_b1_percent")
