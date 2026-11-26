package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test

class SimulatorTest {
    private val now = Instant.parse("2026-11-17T12:00:00Z")

    @Test fun idleAndCruiseUseTheSharedUnits() {
        val idle = Simulator().sample(0, now)
        assertEquals(0.0, idle.speedKmh!!, 0.0)
        assertEquals(850.0, idle.rpm!!, 0.0)
        assertEquals(78.0, idle.coolantC!!, 0.0)
        val cruise = Simulator().sample(30_000, now)
        assertEquals(80.0, cruise.speedKmh!!, 0.0)
        assertTrue(cruise.rpm!! in 3600.0..3700.0)
        assertEquals("connected", cruise.health)
    }

    @Test fun missingVoltageIsNullAndNotZero() {
        assertNotNull(Simulator().sample(23_999, now).voltageV)
        assertNull(Simulator().sample(24_000, now).voltageV)
        assertNull(Simulator().sample(27_999, now).voltageV)
        assertNotNull(Simulator().sample(28_000, now).voltageV)
    }

    @Test fun staleAndDisconnectedPhasesHideAllNumbersAndRecover() {
        val sim = Simulator()
        assertEquals("connected", sim.sample(41_999, now).health)
        for (elapsed in listOf(42_000L, 47_999L, 48_000L, 53_999L)) {
            val sample = sim.sample(elapsed, now)
            assertEquals(if (elapsed < 48_000) "stale" else "disconnected", sample.health)
            assertNull(sample.rpm)
            assertNull(sample.speedKmh)
            assertNull(sample.coolantC)
            assertNull(sample.voltageV)
        }
        assertEquals(now.minusMillis(4000), sim.sample(42_000, now).capturedAt)
        assertNull(sim.sample(48_000, now).capturedAt)
        assertEquals("connected", sim.sample(54_000, now).health)
        assertNotNull(sim.sample(54_000, now).rpm)
        assertEquals(0.0, sim.sample(60_000, now).speedKmh!!, 0.0)
    }

    @Test fun monotonicElapsedDrivesPhaseIndependentlyOfWallClock() {
        val sim = Simulator()
        assertEquals(sim.sample(10_000, now).speedKmh, sim.sample(10_000, now.minusSeconds(500)).speedKmh)
        assertEquals(0.0, sim.sample(-1, now).speedKmh!!, 0.0)
    }

    @Test fun wireContractAlwaysIdentifiesSimulationAndUsesExplicitNulls() {
        val sim = Simulator()
        val json = JSONObject(sim.sample(30_000, now).json())
        assertEquals("simulator", json.getString("source"))
        assertEquals("connected", json.getString("health"))
        assertEquals(80.0, json.getDouble("speed_kmh"), 0.0)
        assertEquals("2026-11-17T12:00:00Z", json.getString("captured_at"))
        assertEquals(json.getString("captured_at"), json.getString("received_at"))
        assertEquals(json.getString("captured_at"), json.getString("server_time"))
        assertEquals(3000, json.getInt("stale_after_ms"))
        assertFalse(json.has("fuel"))
        val disconnected = JSONObject(sim.sample(49_000, now).json())
        for (key in listOf("rpm", "speed_kmh", "coolant_c", "voltage_v", "captured_at")) {
            assertTrue("explicit null: $key", disconnected.has(key) && disconnected.isNull(key))
        }
        assertEquals("simulator", disconnected.getString("source"))
    }
}
