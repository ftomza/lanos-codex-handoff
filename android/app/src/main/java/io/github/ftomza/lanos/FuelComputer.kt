package io.github.ftomza.lanos

import java.io.IOException
import java.time.Instant
import java.util.UUID
import org.json.JSONObject

internal data class FuelCalibration(
    val token: String,
    val revision: Long,
    val liters: Double,
    val factor: Double,
    val previous: Double,
)

internal class FuelComputer(
    private val mode: FuelMode,
    private val store: FuelStore,
    tick: Long,
    now: Instant,
) {
    private data class Endpoint(
        val tick: Long,
        val generation: Long,
        val estimate: FuelEstimate,
        val speed: FuelReading?,
    ) {
        val flow get() = estimate.flow
    }

    var state = FuelState(mode, now)
        private set
    private var latest: Endpoint? = null
    private var previous: Endpoint? = null
    private var pending: FuelCalibration? = null
    private var lastSave = tick
    private var dirty = true
    private var loadError: IOException? = null
    private var saveError: IOException? = null

    init {
        try {
            store.load()?.validate(mode)?.let { state = it.copy(reasons = it.reasons + "process_restart") }
        } catch (e: Exception) {
            loadError = IOException("Файл поездки повреждён или несовместим", e)
        }
        flush(tick, now)
    }

    fun observe(tick: Long, now: Instant, inputs: FuelInputs) {
        val next = Endpoint(
            tick, inputs.generation, estimateFuel(mode, inputs, tick),
            inputs.speed?.takeIf { inputs.connected && it.valid(tick, 0.0..220.0) },
        )
        latest = next
        if (loadError != null) return
        val prev = previous
        if (prev == null) {
            if (next.flow != null || next.speed != null) previous = next
        } else {
            previous = next
            val elapsed = next.tick - prev.tick
            if (elapsed > 0) advance(prev, next, elapsed)
            if (elapsed < 0) mark("clock_reversal")
        }
        if (dirty && tick - lastSave >= 5000) flush(tick, now)
    }

    private fun advance(prev: Endpoint, next: Endpoint, elapsed: Long) {
        val same = elapsed <= 3000 && prev.generation == next.generation
        val liters = if (same) integrate(prev.flow, next.flow, next.tick, elapsed) else null
        val km = if (same) integrate(prev.speed, next.speed, next.tick, elapsed) else null
        val joint = liters != null && km != null
        state = state.copy(
            baseLiters = state.baseLiters + (liters ?: 0.0),
            jointLiters = state.jointLiters + (if (joint) liters else 0.0),
            distanceKm = state.distanceKm + (km ?: 0.0),
            coveredKm = state.coveredKm + (if (joint) km else 0.0),
            observedMs = state.observedMs + elapsed,
            coveredMs = state.coveredMs + (if (joint) elapsed else 0),
            gapMs = state.gapMs + (if (joint) 0 else elapsed),
        )
        if (!joint) mark(if (same) "unavailable_input" else "timing_or_generation_gap")
        dirty = true
    }

    private fun integrate(a: FuelReading?, b: FuelReading?, tick: Long, elapsed: Long): Double? {
        if (a == null || b == null || tick >= a.until) return null
        return (a.value + b.value) / 2 * (elapsed / 3_600_000.0)
    }

    private fun mark(reason: String) {
        state = state.copy(reasons = state.reasons + reason)
        pending = null
        dirty = true
    }

    fun suspendTrip(tick: Long, now: Instant, reason: String) {
        if (previous != null || state.observedMs > 0) mark(reason)
        previous = null
        latest = null
        pending = null
        if (dirty) flush(tick, now)
    }

    private fun flush(tick: Long, now: Instant) {
        if (loadError != null) return
        lastSave = tick
        try {
            save(state, now)
        } catch (_: IOException) {
            // The view reports the error. A later tick can retry.
        }
    }

    private fun save(next: FuelState, now: Instant) {
        loadError?.let { throw it }
        val stamped = next.copy(savedAt = now).validate(mode)
        try {
            store.save(stamped)
        } catch (e: IOException) {
            val failure = IOException("Не удалось записать файл поездки", e)
            saveError = failure
            throw failure
        }
        state = stamped
        saveError = null
        dirty = false
    }

    private fun requireRevision(expected: Long) {
        loadError?.let { throw it }
        require(state.revision == expected) { "Поездка изменилась. Откройте диалог заново." }
    }

    fun preview(revision: Long, liters: Double, confirmed: Boolean): FuelCalibration {
        requireRevision(revision)
        require(confirmed && liters in 0.01..1000.0 && !state.incomplete &&
            state.coveredKm >= 10 && state.baseLiters > 0) {
            "Нужны совпадающие границы от полного до полного, полное покрытие и от 10 км."
        }
        val factor = liters / state.baseLiters
        require(factor in FuelState.factorRange) { "Поправка должна быть от 0,25 до 4." }
        return FuelCalibration(UUID.randomUUID().toString(), revision, liters, factor, state.factor)
            .also { pending = it }
    }

    fun reset(revision: Long, defaultFactor: Boolean, tick: Long, now: Instant) {
        requireRevision(revision)
        val next = FuelState(
            mode, now, revision = revision,
            factor = if (defaultFactor) 1.0 else state.factor,
            previousFactor = if (defaultFactor) state.factor else state.previousFactor,
        )
        commit(next, tick, now)
        // The user fixed the start of this trip even if there is no telemetry yet.
        previous = latest?.takeIf { it.tick == tick }
            ?: Endpoint(tick, latest?.generation ?: 0, FuelEstimate(), null)
    }

    fun apply(
        revision: Long, action: String, liters: Double?, confirmed: Boolean,
        token: String?, tick: Long, now: Instant,
    ) {
        requireRevision(revision)
        val factor = when (action) {
            "apply" -> {
                val preview = pending
                require(confirmed && preview != null && preview.token == token &&
                    preview.revision == revision && preview.liters == liters && !state.incomplete) {
                    "Рассчитайте поправку заново и подтвердите замер."
                }
                preview.factor
            }
            "undo" -> requireNotNull(state.previousFactor) { "Предыдущей поправки нет." }
            "default" -> 1.0
            else -> throw IllegalArgumentException("Неизвестное действие.")
        }
        commit(state.copy(factor = factor, previousFactor = state.factor), tick, now)
    }

    private fun commit(next: FuelState, tick: Long, now: Instant) {
        save(next.copy(revision = state.revision + 1), now)
        pending = null
        lastSave = tick
    }

    fun view(tick: Long): JSONObject {
        val sample = latest
        val flow = sample?.flow?.takeIf { tick < it.until }
        val speed = sample?.speed?.takeIf { tick < it.until }
        val hasTotals = loadError == null
        val average = if (hasTotals && state.coveredKm >= 0.1)
            state.jointLiters * state.factor * 100 / state.coveredKm else null
        val persistence = when {
            loadError != null || saveError != null -> "persistence_error"
            dirty -> "pending"
            else -> "saved"
        }
        return JSONObject()
            .put("method", "speed_density").put("confidence", "low")
            .put("status", if (flow != null) "available" else "unavailable")
            .put("reason", if (sample?.flow != null && flow == null) "expired" else sample?.estimate?.reason ?: "source_unavailable")
            .put("settings", JSONObject().put("unverified_iat", mode == FuelMode.Experimental))
            .put("flow_lph", flow?.let { it.value * state.factor } ?: JSONObject.NULL)
            .put("flow_remaining_ms", flow?.let { it.until - tick } ?: 0L)
            .put("speed_kmh", speed?.value ?: JSONObject.NULL)
            .put("speed_remaining_ms", speed?.let { it.until - tick } ?: 0L)
            .put("low_speed_kmh", 5).put("average_l100", average ?: JSONObject.NULL)
            .put("liters", if (hasTotals) state.baseLiters * state.factor else JSONObject.NULL)
            .put("distance_km", if (hasTotals) state.distanceKm else JSONObject.NULL)
            .put("covered_km", if (hasTotals) state.coveredKm else JSONObject.NULL)
            .put("incomplete", state.incomplete).put("factor", state.factor)
            .put("previous_factor", state.previousFactor ?: JSONObject.NULL)
            .put("revision", if (hasTotals) state.revision else JSONObject.NULL)
            .put("persistence", persistence)
            .put("persistence_error_label", loadError?.message ?: saveError?.message ?: JSONObject.NULL)
    }
}
