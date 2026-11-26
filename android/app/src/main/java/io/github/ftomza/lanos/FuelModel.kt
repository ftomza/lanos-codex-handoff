package io.github.ftomza.lanos

internal enum class FuelMode {
    Simulator, Live, Experimental,
}

internal data class FuelReading(val value: Double, val at: Long, val ttl: Long) {
    val until get() = at + ttl

    fun valid(tick: Long, range: ClosedFloatingPointRange<Double>): Boolean =
        tick >= at && tick < until && value in range
}

internal data class FuelInputs(
    val connected: Boolean,
    val generation: Long,
    val rpm: FuelReading? = null,
    val map: FuelReading? = null,
    val iat: FuelReading? = null,
    val speed: FuelReading? = null,
)

internal data class FuelEstimate(val flow: FuelReading? = null, val reason: String = "")

internal object SpeedDensity {
    const val displacementLiters = 1.5
    const val volumetricEfficiency = 0.75
    const val airFuelRatio = 14.7
    const val fuelDensityGramsPerLiter = 745
    val stateKey = "speed_density:$displacementLiters:$volumetricEfficiency:$airFuelRatio:$fuelDensityGramsPerLiter"
}

internal fun estimateFuel(mode: FuelMode, inputs: FuelInputs, tick: Long): FuelEstimate {
    if (!inputs.connected) return FuelEstimate(reason = "source_unavailable")
    val rpm = inputs.rpm?.takeIf { it.valid(tick, 0.0..7000.0) }
        ?: return FuelEstimate(reason = "rpm_unavailable")
    if (rpm.value == 0.0) return FuelEstimate(rpm)
    if (mode == FuelMode.Live) return FuelEstimate(reason = "unverified_iat")
    val map = inputs.map?.takeIf { it.valid(tick, 0.001..120.0) }
        ?: return FuelEstimate(reason = "map_unavailable")
    val iat = inputs.iat?.takeIf { it.valid(tick, -29.999..100.0) }
        ?: return FuelEstimate(reason = "iat_unavailable_or_out_of_range")
    val density = map.value * 1000 / (287.05 * (iat.value + 273.15))
    val airGramsPerSecond = density * (SpeedDensity.displacementLiters / 1000) *
        (rpm.value / 120) * SpeedDensity.volumetricEfficiency * 1000
    val flow = airGramsPerSecond * 3600 / (SpeedDensity.airFuelRatio * SpeedDensity.fuelDensityGramsPerLiter)
    if (flow !in 0.0..100.0) return FuelEstimate(reason = "flow_out_of_range")
    val until = minOf(rpm.until, map.until, iat.until)
    return FuelEstimate(FuelReading(flow, tick, until - tick))
}
