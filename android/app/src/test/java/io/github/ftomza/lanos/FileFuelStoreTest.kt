package io.github.ftomza.lanos

import java.time.Instant
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class FileFuelStoreTest {
    @get:Rule val folder = TemporaryFolder()

    @Test fun trailingContentAndFractionalVersionCannotBeOverwritten() {
        val now = Instant.parse("2026-11-23T00:00:00Z")
        val file = folder.root.resolve("fuel-simulator.json")
        val store = FileFuelStore(folder.root, FuelMode.Simulator)
        store.save(FuelState(FuelMode.Simulator, now, baseLiters = 2.0))
        val original = file.readText()
        for (invalid in listOf(original + "garbage", original.replace("\"version\":1", "\"version\":1.5"))) {
            file.writeText(invalid)
            val computer = FuelComputer(FuelMode.Simulator, store, 0, now)
            assertEquals("persistence_error", computer.view(0).getString("persistence"))
            assertEquals(invalid, file.readText())
        }
    }

    @Test fun persistedModesAreIndependentAndKeepRawTotalsAndFactors() {
        val now = Instant.parse("2026-11-23T00:00:00Z")
        val sim = FileFuelStore(folder.root, FuelMode.Simulator)
        val live = FileFuelStore(folder.root, FuelMode.Live)
        val experimental = FileFuelStore(folder.root, FuelMode.Experimental)
        val s = FuelState(FuelMode.Simulator, now, baseLiters = 2.0, factor = 1.5, previousFactor = 1.2)
        val e = FuelState(FuelMode.Experimental, now, baseLiters = 3.0, factor = 0.8)
        sim.save(s)
        experimental.save(e)
        assertEquals(s, sim.load())
        assertEquals(e, experimental.load())
        assertNull(live.load())

        val file = folder.root.resolve("fuel-simulator.json")
        val original = file.readText()
        file.writeText(original.replace("\"version\":1", "\"version\":2"))
        val computer = FuelComputer(FuelMode.Simulator, sim, 0, now)
        assertEquals("persistence_error", computer.view(0).getString("persistence"))
        assertTrue(file.readText().contains("\"version\":2"))
    }
}
