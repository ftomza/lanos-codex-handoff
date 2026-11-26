package io.github.ftomza.lanos

import java.io.File
import java.io.IOException
import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class FuelComputerTest {
    private val now = Instant.parse("2026-11-23T00:00:00Z")

    @Test fun speedDensityMatchesSharedGoVectors() {
        val cases = golden().getJSONArray("estimates")
        for (i in 0 until cases.length()) {
            val row = cases.getJSONObject(i)
            val mode = when {
                row.optBoolean("allow") -> FuelMode.Experimental
                row.optBoolean("live") -> FuelMode.Live
                else -> FuelMode.Simulator
            }
            val result = estimateFuel(mode, row.inputs(), row.optLong("age_ms"))
            val name = row.getString("name")
            if (row.has("flow")) assertEquals(name, row.getDouble("flow"), result.flow!!.value, 1e-9)
            else {
                assertNull(name, result.flow)
                assertEquals(name, row.getString("reason"), result.reason)
            }
        }
    }

    @Test fun totalsMatchSharedGoVectorsAcrossMissingInputsAndGenerationChanges() {
        val fixture = golden()
        val computer = FuelComputer(FuelMode.Simulator, MemoryFuelStore(), 0, now)
        val trip = fixture.getJSONArray("trip")
        for (i in 0 until trip.length()) {
            val row = trip.getJSONObject(i)
            val ms = row.getLong("ms")
            computer.observe(ms, now.plusMillis(ms), row.inputs(ms))
        }
        val s = computer.state
        val expected = fixture.getJSONObject("totals")
        for ((key, value) in mapOf("liters" to s.baseLiters, "joint_liters" to s.jointLiters,
            "km" to s.distanceKm, "covered_km" to s.coveredKm, "observed_s" to s.observedMs / 1000.0,
            "covered_s" to s.coveredMs / 1000.0, "gap_s" to s.gapMs / 1000.0)) {
            assertEquals(key, expected.getDouble(key), value, 1e-9)
        }
        assertTrue(s.incomplete)
    }

    @Test fun renewedSensorCannotBridgeItsOldDeadline() {
        val computer = FuelComputer(FuelMode.Simulator, MemoryFuelStore(), 0, now)
        computer.observe(0, now, running(0, 1000))
        computer.observe(1000, now, running(1000))
        assertTrue(computer.state.incomplete)
        assertEquals(0.0, computer.state.baseLiters, 0.0)
        assertEquals(0.02, computer.state.distanceKm, 1e-9)
    }

    @Test fun calibrationFreezesFactorAndFailedWritesDoNotChangeTrip() {
        val store = MemoryFuelStore()
        val computer = FuelComputer(FuelMode.Simulator, store, 0, now)
        for (second in 0L..501L) computer.observe(second * 1000, now, running(second * 1000))
        val old = computer.state
        val measured = old.baseLiters * 1.5
        val preview = computer.preview(old.revision, measured, true)
        computer.observe(502_000, now, running(502_000))
        store.failSave = true
        assertThrows(IOException::class.java) {
            computer.apply(old.revision, "apply", measured, true, preview.token, 502_000, now)
        }
        assertEquals(1.0, computer.state.factor, 0.0)
        assertThrows(IOException::class.java) { computer.reset(old.revision, false, 502_000, now) }
        assertTrue(computer.state.distanceKm > old.distanceKm)
        store.failSave = false
        computer.apply(old.revision, "apply", measured, true, preview.token, 502_000, now)
        assertEquals(1.5, computer.state.factor, 1e-9)
        assertThrows(IllegalArgumentException::class.java) { computer.reset(old.revision, false, 502_000, now) }
        computer.reset(computer.state.revision, false, 502_000, now)
        assertEquals(0.0, computer.state.baseLiters, 0.0)
        assertEquals(1.5, computer.state.factor, 1e-9)
        computer.apply(computer.state.revision, "undo", null, false, null, 502_000, now)
        assertEquals(1.0, computer.state.factor, 0.0)
    }

    @Test fun reloadMarksGapWithoutRestoringEndpointAndRejectsWrongMode() {
        val store = MemoryFuelStore()
        val first = FuelComputer(FuelMode.Simulator, store, 0, now)
        first.observe(0, now, running(0))
        first.observe(1000, now, running(1000))
        first.suspendTrip(1000, now, "background")
        val restored = FuelComputer(FuelMode.Simulator, store, 5000, now)
        restored.observe(5000, now, running(5000))
        assertTrue("process_restart" in restored.state.reasons)
        assertEquals(0.02, restored.state.distanceKm, 1e-9)
        val wrongMode = FuelComputer(FuelMode.Experimental, store, 5000, now)
        assertEquals("persistence_error", wrongMode.view(5000).getString("persistence"))
        assertThrows(IOException::class.java) { wrongMode.reset(wrongMode.state.revision, false, 5000, now) }
        assertEquals(FuelMode.Simulator, store.saved!!.mode)
    }

    @Test fun gapInvalidatesCalibrationEvenWhenRevisionIsUnchanged() {
        val computer = FuelComputer(FuelMode.Simulator, MemoryFuelStore(), 0, now)
        for (second in 0L..501L) computer.observe(second * 1000, now, running(second * 1000))
        val old = computer.state
        val preview = computer.preview(old.revision, old.baseLiters, true)
        computer.observe(505_000, now, running(505_000))
        assertThrows(IllegalArgumentException::class.java) {
            computer.apply(old.revision, "apply", old.baseLiters, true, preview.token, 505_000, now)
        }
    }

    @Test fun resetWithNoDataCannotHideAMissingStartOfTrip() {
        val computer = FuelComputer(FuelMode.Experimental, MemoryFuelStore(), 0, now)
        computer.observe(0, now, FuelInputs(false, 1))
        computer.reset(computer.state.revision, false, 0, now)
        for (second in 1L..503L) computer.observe(second * 1000, now, running(second * 1000))
        assertTrue(computer.state.coveredKm > 10)
        assertTrue(computer.state.incomplete)
        assertThrows(IllegalArgumentException::class.java) {
            computer.preview(computer.state.revision, computer.state.baseLiters, true)
        }
    }

    @Test fun invalidPersistedStateIsHeldForRecovery() {
        val store = MemoryFuelStore().apply { failLoad = true }
        val computer = FuelComputer(FuelMode.Live, store, 0, now)
        computer.observe(6000, now, running(6000))
        assertEquals("persistence_error", computer.view(6000).getString("persistence"))
        assertThrows(IOException::class.java) { computer.reset(computer.state.revision, false, 6000, now) }
        assertNull(store.saved)
    }

    private fun golden() = JSONObject(File(checkNotNull(System.getProperty("lanos.fuel.fixture"))).readText())

    private fun JSONObject.inputs(ms: Long = 0): FuelInputs {
        fun value(key: String) = if (isNull(key)) null else FuelReading(getDouble(key), ms, 3000)
        return FuelInputs(true, optLong("generation", 1), value("rpm"), value("map"), value("iat"), value("speed"))
    }

    private fun running(at: Long, rpmTtl: Long = 3000) = FuelInputs(
        true, 1, FuelReading(2400.0, at, rpmTtl), FuelReading(40.0, at, 3000),
        FuelReading(20.0, at, 3000), FuelReading(72.0, at, 3000),
    )

    private class MemoryFuelStore : FuelStore {
        var saved: FuelState? = null
        var failSave = false
        var failLoad = false
        override fun load(): FuelState? {
            if (failLoad) throw IOException("invalid state")
            return saved
        }
        override fun save(state: FuelState) {
            if (failSave) throw IOException("disk full")
            saved = state
        }
    }
}
