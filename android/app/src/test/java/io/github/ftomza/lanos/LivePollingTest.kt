package io.github.ftomza.lanos

import java.io.File
import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class LivePollingTest {
    @Test fun decodesTheSamePhysicalResponsesAsGo() {
        val evidence = JSONObject(File(checkNotNull(System.getProperty("lanos.ble.fixture"))).readText())
            .getJSONArray("observations")
        fun raw(command: ElmCommand, contains: String): String = (0 until evidence.length())
            .asSequence().map(evidence::getJSONObject)
            .first { it.getString("command") == command.wire && it.getString("raw").contains(contains) }
            .getString("raw")

        val f = Fixture()
        assertTrue(f.reply(ElmCommand.Capabilities, raw(ElmCommand.Capabilities, "BE 3E B8 11")))
        assertTrue(f.reply(ElmCommand.Rpm, raw(ElmCommand.Rpm, "0D F2")))
        assertTrue(f.reply(ElmCommand.Coolant, raw(ElmCommand.Coolant, "5A")))
        assertTrue(f.reply(ElmCommand.Voltage, raw(ElmCommand.Voltage, "11.0V")))
        val snapshot = f.snapshot()
        assertEquals("live", snapshot.getString("source"))
        assertEquals(892.5, snapshot.getDouble("rpm"), 0.0)
        assertEquals(50.0, snapshot.getDouble("coolant_c"), 0.0)
        assertEquals(11.0, snapshot.getDouble("voltage_v"), 0.0)
        assertTrue(snapshot.isNull("speed_kmh"))
    }

    @Test fun validZeroAndSeparateLifetimesSurviveOtherReplies() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "41 00 BE 3E B8 11\r>")
        f.reply(ElmCommand.Rpm, "010C\r41 0C 00 00\r>")
        f.reply(ElmCommand.Coolant, "410550\r>")
        f.reply(ElmCommand.Voltage, "14.2V\r>")
        assertEquals(0.0, f.snapshot().getDouble("rpm"), 0.0)
        f.advance(3000)
        f.reply(ElmCommand.Voltage, "13.8v\r>")
        var snapshot = f.snapshot()
        assertTrue(snapshot.isNull("rpm"))
        assertEquals("stale", snapshot.getJSONObject("signals").getJSONObject("rpm").getString("status"))
        assertEquals(40.0, snapshot.getDouble("coolant_c"), 0.0)
        assertEquals(13.8, snapshot.getDouble("voltage_v"), 0.0)
        assertEquals("2026-11-23T12:00:00Z",
            snapshot.getJSONObject("signals").getJSONObject("coolant_c").getString("captured_at"))
        f.advance(7000)
        snapshot = f.snapshot()
        assertTrue(snapshot.isNull("coolant_c"))
        assertEquals(13.8, snapshot.getDouble("voltage_v"), 0.0)
    }

    @Test fun voltageWorksWithoutEcuAndUnsupportedPidsAreNotRequested() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "UNABLE TO CONNECT\r>")
        assertEquals(listOf(ElmCommand.Voltage), f.polling.commands())
        f.reply(ElmCommand.Voltage, "12.7V\r>")
        assertEquals("connected", f.snapshot().getString("health"))
        assertEquals(12.7, f.snapshot().getDouble("voltage_v"), 0.0)
        assertTrue(f.snapshot().isNull("rpm"))
        f.reply(ElmCommand.Capabilities, "410008000000\r>")
        assertEquals(listOf(ElmCommand.Coolant, ElmCommand.Voltage), f.polling.commands())
        assertEquals("unsupported", f.snapshot().getJSONObject("signals").getJSONObject("rpm").getString("support"))
    }

    @Test fun completedNegativeAndAmbiguousRepliesRemoveOldValues() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "4100BE3EB811\r>")
        f.reply(ElmCommand.Rpm, "SEARCHING... 410C0DF2\r410C0DF2\r>")
        assertEquals(892.5, f.snapshot().getDouble("rpm"), 0.0)
        for (bad in listOf("NO DATA", "STOPPED", "?", "7F 01 11", "410C0DF2\r410C0D00", "410500", "410C0D")) {
            f.reply(ElmCommand.Rpm, "410C0DF2\r>")
            f.reply(ElmCommand.Rpm, "$bad\r>")
            assertTrue(bad, f.snapshot().isNull("rpm"))
        }
        f.reply(ElmCommand.Voltage, "14.2V\r>")
        f.reply(ElmCommand.Voltage, "14.2\r>")
        assertTrue(f.snapshot().isNull("voltage_v"))
        assertFalse(f.reply(ElmCommand.EchoOff, "?\r>"))
    }

    @Test fun adapterVoltageUsesTheSameInclusiveRangeAsGo() {
        val f = Fixture()
        for (invalid in listOf("32.1V", "99.9V")) {
            f.reply(ElmCommand.Voltage, "32.0V\r>")
            assertEquals(32.0, f.snapshot().getDouble("voltage_v"), 0.0)
            f.reply(ElmCommand.Voltage, "$invalid\r>")
            assertTrue(f.snapshot().isNull("voltage_v"))
        }
    }

    @Test fun aBackwardClockStepCannotIncreaseRemainingLifetime() {
        val f = Fixture()
        f.reply(ElmCommand.Rpm, "410C0DF2\r>")
        f.advance(2900)
        f.now = f.now.minusSeconds(1)
        val snapshot = f.snapshot()
        assertEquals(100L, snapshot.getLong("remaining_ms"))
        assertEquals(100L, snapshot.getJSONObject("signals").getJSONObject("rpm").getLong("remaining_ms"))
    }

    @Test fun fuelInputsAreFreshAndDoNotSurviveEcuStandby() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "4100BE3EB811\r>")
        f.reply(ElmCommand.Rpm, "410C0DF2\r>")
        f.reply(ElmCommand.Map, "410B25\r>")
        f.reply(ElmCommand.Iat, "410F3C\r>")
        f.reply(ElmCommand.Speed, "410D00\r>")
        val live = f.polling.fuelInputs(ProbeStage.Live, f.tick)
        assertEquals(892.5, live.rpm!!.value, 0.0)
        assertEquals(37.0, live.map!!.value, 0.0)
        assertEquals(20.0, live.iat!!.value, 0.0)
        assertEquals(0.0, live.speed!!.value, 0.0)
        f.advance(3000)
        assertNull(f.polling.fuelInputs(ProbeStage.Live, f.tick).rpm)
        f.polling.clearEcu()
        val standby = f.polling.fuelInputs(ProbeStage.WaitingEcu, f.tick)
        assertFalse(standby.connected)
        assertTrue(standby.generation != live.generation)
        assertNull(standby.iat)
    }

    @Test fun journalRecordsMonotonicAgeWithoutStaleValuesOrRawMessages() {
        val fixture = Fixture()
        fixture.reply(ElmCommand.Capabilities, "4100BE3EB811\r>")
        fixture.reply(ElmCommand.Rpm, "410C0DF2\r>")
        fixture.reply(ElmCommand.Map, "410B25\r>")
        fixture.reply(ElmCommand.Iat, "410F3C\r>")
        fixture.reply(ElmCommand.Speed, "410D00\r>")
        fixture.advance(3000)
        val result = ProbeResult(ProbeStage.Live, "synthetic raw response")
        val input = fixture.polling.fuelInputs(result.stage, fixture.tick)
        val computer = FuelComputer(FuelMode.Experimental, object : FuelStore {
            override fun load(): FuelState? = null
            override fun save(state: FuelState) = Unit
        }, fixture.tick, fixture.now)
        computer.observe(fixture.tick, fixture.now, input)
        val logged = tripSample(fixture.polling, result, fixture.tick, fixture.now, computer)
        val signals = logged.getJSONObject("signals")
        val rpm = signals.getJSONObject("rpm")
        assertTrue(rpm.isNull("value"))
        assertEquals("stale", rpm.getString("status"))
        assertEquals(3000L, rpm.getLong("age_ms"))
        assertEquals(20.0, signals.getJSONObject("iat_c").getDouble("value"), 0.0)
        assertEquals("rpm_unavailable", logged.getJSONObject("fuel").getString("reason"))
        assertFalse(logged.toString().contains("synthetic raw response"))
    }

    private class Fixture {
        val polling = LivePolling()
        var tick = 0L
        var now = Instant.parse("2026-11-23T12:00:00Z")
        fun reply(command: ElmCommand, raw: String): Boolean {
            val frame = ElmFrame()
            assertTrue(frame.append(raw.toByteArray(Charsets.US_ASCII)))
            assertTrue(frame.complete)
            return polling.reply(command, frame, tick, now)
        }

        fun advance(ms: Long) {
            tick += ms
            now = now.plusMillis(ms)
        }

        fun snapshot() = JSONObject(polling.json(ProbeResult(ProbeStage.Live), tick, now))
    }
}
