package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LiveScheduleTest {
    @Test fun decodesAdvertisedFuelInputsWithSeparateTimestampsAndNulls() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "4100BE3EB811")
        f.reply(ElmCommand.Map, "410B29")
        f.reply(ElmCommand.Iat, "410F39")
        f.reply(ElmCommand.ShortTrim, "410680")
        f.reply(ElmCommand.LongTrim, "410773")
        val snapshot = f.snapshot()
        val signals = snapshot.getJSONObject("signals")
        assertEquals(41.0, signals.getJSONObject("map_kpa").getDouble("value"), 0.0)
        assertEquals(17.0, signals.getJSONObject("iat_c").getDouble("value"), 0.0)
        assertEquals(0.0, signals.getJSONObject("stft_b1_percent").getDouble("value"), 0.0)
        assertEquals(-10.15625, signals.getJSONObject("ltft_b1_percent").getDouble("value"), 0.0)
        f.tick += 4000
        f.reply(ElmCommand.Iat, "410F00")
        assertEquals(-40.0, f.snapshot().getJSONObject("signals").getJSONObject("iat_c").getDouble("value"), 0.0)
        f.reply(ElmCommand.Iat, "NO DATA")
        assertTrue(f.snapshot().getJSONObject("signals").getJSONObject("iat_c").isNull("value"))
        assertEquals("temporarily_unavailable", f.snapshot().getJSONObject("signals").getJSONObject("iat_c").getString("status"))
        assertEquals("stale", f.snapshot().getJSONObject("signals").getJSONObject("map_kpa").getString("status"))
    }

    @Test fun schedulerFavorsDueCoreItemsAndNeverPollsAnUnsupportedInput() {
        val f = Fixture()
        f.reply(ElmCommand.Capabilities, "410000180000")
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, response) in setup) {
            assertEquals(command, next.command)
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, response))
        }

        val choices = mutableListOf<ElmCommand>()
        repeat(20) {
            f.tick += next.delayMs
            val command = next.command
            choices.add(command)
            val response = when (command) {
                ElmCommand.Rpm -> "410C0DF2"
                ElmCommand.Speed -> "410D00"
                ElmCommand.Voltage -> "13.8V"
                else -> error("unexpected write $command")
            }
            next = scheduler.completed(command, f.reply(command, response))
        }
        assertEquals(ElmCommand.Rpm, choices.first())
        assertTrue(choices.count { it == ElmCommand.Rpm } > choices.count { it == ElmCommand.Voltage })
        assertTrue(ElmCommand.Voltage in choices)
        assertFalse(ElmCommand.Map in choices)
    }

    @Test fun silentEcuIsCheckedSparselyAndPollingResumesOnFreshCapabilityEvidence() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, response) in setup.take(5)) {
            assertEquals(command, next.command)
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Capabilities) "NO DATA" else response))
        }
        assertEquals(LiveSchedule.Phase.Standby, scheduler.phase)
        val entered = f.tick
        while (next.command == ElmCommand.Voltage) {
            f.tick += next.delayMs
            next = scheduler.completed(ElmCommand.Voltage, f.reply(ElmCommand.Voltage, "12.7V"))
        }
        assertEquals(ElmCommand.Capabilities, next.command)
        assertTrue(f.tick + next.delayMs - entered >= 10_000)
        assertEquals(12.7, f.snapshot(ProbeStage.WaitingEcu).getDouble("voltage_v"), 0.0)
        assertTrue(f.snapshot(ProbeStage.WaitingEcu).isNull("rpm"))
        assertEquals("ecu", f.snapshot(ProbeStage.WaitingEcu).getString("standby_reason"))

        f.tick += next.delayMs
        next = scheduler.completed(ElmCommand.Capabilities, f.reply(ElmCommand.Capabilities, "410000100000"))
        assertEquals(ElmCommand.Protocol, next.command)
        f.tick += next.delayMs
        next = scheduler.completed(ElmCommand.Protocol, f.reply(ElmCommand.Protocol, "A5"))
        assertEquals(ElmCommand.Rpm, next.command)
        assertEquals(LiveSchedule.Phase.Poll, scheduler.phase)
        assertTrue(f.snapshot().isNull("rpm"))
    }

    @Test fun anAnsweringEcuWithNoRecognizedPidsKeepsTheUnsupportedStatuses() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, raw) in setup) {
            assertEquals(command, next.command)
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Capabilities) "410040000000" else raw))
        }
        assertEquals(LiveSchedule.Phase.Poll, scheduler.phase)
        val start = f.tick
        repeat(16) {
            assertTrue(next.command == ElmCommand.Voltage || next.command == ElmCommand.Capabilities)
            f.tick += next.delayMs
            if (next.command == ElmCommand.Capabilities) assertTrue(f.tick - start >= 10_000)
            val command = next.command
            val raw = if (command == ElmCommand.Voltage) "12.5V" else "410040000000"
            next = scheduler.completed(command, f.reply(command, raw))
            assertEquals(LiveSchedule.Phase.Poll, scheduler.phase)
        }
        assertEquals("unsupported", f.snapshot().getJSONObject("signals").getJSONObject("rpm").getString("support"))
        assertEquals(12.5, f.snapshot().getDouble("voltage_v"), 0.0)
    }

    @Test fun slowRepliesToAllEightInputsDoNotStarveTheFastInstruments() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, raw) in setup) {
            assertEquals(command, next.command)
            f.tick += next.delayMs + 350
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Capabilities) "4100BE3EB811" else raw))
        }
        val fast = setOf(ElmCommand.Rpm, ElmCommand.Map, ElmCommand.Speed)
        val last = mutableMapOf<ElmCommand, Long>()
        repeat(64) {
            val command = next.command
            f.tick += next.delayMs + 350
            val previous = last.put(command, f.tick)
            if (previous != null && command in fast) {
                assertTrue("$command gap = ${f.tick - previous}", f.tick - previous < 3000L)
            }
            val raw = when (command) {
                ElmCommand.Rpm -> "410C0DF2"
                ElmCommand.Map -> "410B29"
                ElmCommand.Speed -> "410D00"
                ElmCommand.Coolant -> "41055A"
                ElmCommand.Voltage -> "13.0V"
                ElmCommand.Iat -> "410F39"
                ElmCommand.ShortTrim -> "410680"
                ElmCommand.LongTrim -> "410773"
                ElmCommand.MilStatus -> "41010007E1A1"
                else -> error("Unexpected command $command")
            }
            next = scheduler.completed(command, f.reply(command, raw))
        }
        assertEquals(9, last.size)
    }

    @Test fun milAndDtcShareTheCoreScheduleAndOnlyReadOnceOnSteadyOn() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, raw) in setup) {
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Capabilities) "410080180000" else raw))
        }
        val sent = mutableListOf<ElmCommand>()
        repeat(42) {
            f.tick += next.delayMs
            val command = next.command
            sent += command
            val raw = when (command) {
                ElmCommand.Rpm -> "410C0DF2"
                ElmCommand.Speed -> "410D00"
                ElmCommand.Voltage -> "13.8V"
                ElmCommand.MilStatus -> "41018107E1A1"
                ElmCommand.StoredDtc -> "4304200000"
                ElmCommand.PendingDtc -> "470000"
                else -> error("Unexpected $command")
            }
            next = scheduler.completed(command, f.reply(command, raw))
        }
        val diagnostics = listOf(ElmCommand.MilStatus, ElmCommand.StoredDtc, ElmCommand.PendingDtc)
        assertTrue(sent.count { it == ElmCommand.MilStatus } >= 2)
        assertEquals(diagnostics.drop(1), sent.filter { it == ElmCommand.StoredDtc || it == ElmCommand.PendingDtc })
        assertTrue(sent.zipWithNext().none { (a, b) -> a in diagnostics && b in diagnostics })
        assertEquals("on", f.snapshot().getJSONObject("mil").getString("state"))
        assertEquals("unknown", f.snapshot(ProbeStage.WaitingEcu).getJSONObject("mil").getString("state"))
    }

    @Test fun repeatedOverdueMilDoesNotStarveDtcOrRegularSupport() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, raw) in setup) {
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Capabilities) "410080380000" else raw))
        }
        val sent = mutableListOf<ElmCommand>()
        repeat(50) {
            f.tick += next.delayMs + 1600
            val command = next.command
            sent += command
            val raw = when (command) {
                ElmCommand.Rpm -> "410C0DF2"
                ElmCommand.Speed -> "410D00"
                ElmCommand.Map -> "410B29"
                ElmCommand.Voltage -> "13.8V"
                ElmCommand.MilStatus -> "41018107E1A1"
                ElmCommand.StoredDtc -> "4304200000"
                ElmCommand.PendingDtc -> "470000"
                else -> error("Unexpected $command")
            }
            next = scheduler.completed(command, f.reply(command, raw))
        }
        assertTrue(ElmCommand.StoredDtc in sent)
        assertTrue(ElmCommand.PendingDtc in sent)
        assertTrue(ElmCommand.Voltage in sent)
    }

    @Test fun threeFailedCoreRoundsInvalidatePreviousEcuReadingsAndSparseChecksReinitialize() {
        val f = Fixture()
        val scheduler = LiveSchedule(f.polling)
        var next = scheduler.begin(f.tick)
        for ((command, raw) in setup) {
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, raw))
        }
        var zeroReads = 0
        for (step in 0 until 20) {
            val command = next.command
            f.tick += next.delayMs
            if (command == ElmCommand.Rpm) zeroReads++
            val raw = when (command) {
                ElmCommand.Voltage -> "12.7V"
                ElmCommand.Rpm -> "410C0000"
                else -> "NO DATA"
            }
            next = scheduler.completed(command, f.reply(command, raw))
            assertEquals(LiveSchedule.Phase.Poll, scheduler.phase)
            if (command == ElmCommand.Rpm && zeroReads >= 3) break
        }
        assertTrue(zeroReads >= 3)
        assertEquals(0.0, f.snapshot().getDouble("rpm"), 0.0)

        var failedRpm = 0
        var failedSpeed = 0
        repeat(30) {
            if (scheduler.phase == LiveSchedule.Phase.Standby) return@repeat
            val command = next.command
            f.tick += next.delayMs
            if (command == ElmCommand.Rpm) failedRpm++
            if (command == ElmCommand.Speed) failedSpeed++
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Voltage) "12.7V" else "NO DATA"))
        }
        assertTrue(failedRpm >= 3)
        assertTrue(failedSpeed >= 3)
        assertEquals(LiveSchedule.Phase.Standby, scheduler.phase)
        assertTrue(f.snapshot(ProbeStage.WaitingEcu).isNull("rpm"))
        var misses = 0
        repeat(20) {
            if (scheduler.phase != LiveSchedule.Phase.Standby) return@repeat
            val command = next.command
            f.tick += next.delayMs
            if (command == ElmCommand.Capabilities) misses++
            next = scheduler.completed(command, f.reply(command, if (command == ElmCommand.Voltage) "12.7V" else "NO DATA"))
        }
        assertEquals(3, misses)
        assertEquals(ElmCommand.EchoOff, next.command)
        val lastProbe = f.tick
        for ((command, raw) in setup.take(4)) {
            assertEquals(command, next.command)
            f.tick += next.delayMs
            next = scheduler.completed(command, f.reply(command, raw))
        }
        assertEquals(ElmCommand.Capabilities, next.command)
        assertTrue(f.tick + next.delayMs - lastProbe >= 10_000L)
    }

    private class Fixture {
        val polling = LivePolling()
        var tick = 0L
        private val start = Instant.parse("2026-11-23T12:00:00Z")

        fun reply(command: ElmCommand, raw: String): Long {
            val frame = ElmFrame()
            assertTrue(frame.append("$raw\r>".toByteArray(Charsets.US_ASCII)))
            assertTrue(polling.reply(command, frame, tick, start.plusMillis(tick)))
            return tick
        }

        fun snapshot(stage: ProbeStage = ProbeStage.Live) =
            JSONObject(polling.json(ProbeResult(stage), tick, start.plusMillis(tick)))
    }

    private companion object {
        val setup = listOf(
            ElmCommand.EchoOff to "OK", ElmCommand.LinefeedsOff to "OK",
            ElmCommand.Identity to "ELM327 v2.3", ElmCommand.AutoProtocol to "OK",
            ElmCommand.Capabilities to "410000180000", ElmCommand.Protocol to "A5",
        )
    }
}
