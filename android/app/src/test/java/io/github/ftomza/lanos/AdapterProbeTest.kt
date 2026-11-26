package io.github.ftomza.lanos

import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothStatusCodes
import java.time.Instant
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AdapterProbeTest {
    @Test fun liveSerializesSetupAndPollsOnlySupportedReadings() {
        val f = Fixture(LivePolling())
        val link = f.startReading()
        val setup = listOf(
            ElmCommand.EchoOff to "OK", ElmCommand.LinefeedsOff to "OK",
            ElmCommand.Identity to "ELM327 v2.3", ElmCommand.AutoProtocol to "OK",
            ElmCommand.Capabilities to "410008100000", ElmCommand.Protocol to "A5",
        )
        for ((index, entry) in setup.withIndex()) {
            val (command, response) = entry
            assertEquals(command, link.commands.last())
            link.receive("$response\r")
            link.emit(GattEvent.Written)
            assertEquals(index + 1, link.writes)
            link.receive(">")
            assertEquals(index + 1, link.writes)
            f.fireTimer()
        }
        assertEquals(ElmCommand.Rpm, link.commands.last())
        link.emit(GattEvent.Written)
        link.receive("410C0DF2\r>")
        assertEquals(ProbeStage.Live, f.result.stage)
        assertFalse(link.closed)
        f.fireTimer()
        assertEquals(ElmCommand.Coolant, link.commands.last())
        link.emit(GattEvent.Written)
        link.receive("41055A\r>")
        f.fireTimer()
        assertEquals(ElmCommand.Rpm, link.commands.last())
        link.emit(GattEvent.Written)
        link.receive("410C0DF2\r>")
        f.fireTimer()
        assertEquals(ElmCommand.Voltage, link.commands.last())
        assertFalse(link.commands.contains(ElmCommand.Speed))
        f.probe.cancel()
        assertTrue(link.closed)
        assertTrue(f.timers.isEmpty())
    }

    @Test fun delayedPromptDuringPassiveWaitAvoidsTheIdentityProbe() {
        val f = Fixture(LivePolling())
        val old = f.startReading()
        old.emit(GattEvent.Written)
        f.probe.cancel()
        assertTrue(old.closed)
        val next = f.startReading()
        assertEquals(ProbeStage.Recovering, f.result.stage)
        assertEquals(0, next.writes)
        old.receive("OK\r>")
        assertEquals(0, next.writes)
        next.receive("OK\r")
        assertEquals(0, next.writes)
        next.receive(">")
        f.fireTimer()
        assertEquals(listOf(ElmCommand.EchoOff), next.commands)
    }

    @Test fun liveRecoversWithoutATapWhenTheOldPromptWasLostOnDisconnect() {
        val f = Fixture(LivePolling())
        val old = f.startReading()
        old.emit(GattEvent.Written)
        old.emit(GattEvent.Failed("Disconnected", retryable = true))
        f.fireTimer()
        val link = f.links.last()
        f.ready(link)
        assertEquals(8000L, f.nextDelay)
        f.fireTimer()
        assertEquals(ProbeStage.Recovering, f.result.stage)
        assertFalse(f.result.recoveryRequired)
        assertFalse(link.closed)
        assertTrue(f.probe.running)
        assertEquals(listOf(ElmCommand.Identity), link.commands)
        old.receive("OK\r>")
        link.receive("OK\r>\rATI\rELM327 v2.3\r")
        assertEquals(listOf(ElmCommand.Identity), link.commands)
        link.receive(">\r\n")
        assertEquals(ProbeStage.Recovering, f.result.stage)
        link.emit(GattEvent.Written)
        assertEquals(ProbeStage.Reading, f.result.stage)
        f.fireTimer()
        assertEquals(listOf(ElmCommand.Identity, ElmCommand.EchoOff), link.commands)
        assertFalse(f.result.recoveryRequired)
    }

    @Test fun wakeProbesOnceAndAPlainPromptCannotUnlockTheNewSession() {
        val f = Fixture(LivePolling())
        val old = f.startReading()
        old.emit(GattEvent.Written)
        f.probe.cancel()
        val link = f.startReading()
        f.fireTimer()
        assertEquals(listOf(ElmCommand.Identity), link.commands)
        link.emit(GattEvent.Written)
        link.receive("NO DATA\r>")
        assertEquals(ProbeStage.Recovering, f.result.stage)
        f.fireTimer()
        assertTrue(f.result.recoveryRequired)
        assertTrue(f.timers.isEmpty())
        link.receive(">")
        assertEquals(listOf(ElmCommand.Identity), link.commands)
        assertTrue(f.timers.isEmpty())
        link.emit(GattEvent.Failed("Disconnected", retryable = true))
        f.fireTimer()
        val next = f.links.last()
        f.ready(next)
        next.receive("OK\r>")
        assertTrue(f.timers.isEmpty())
        assertEquals(0, next.writes)
        assertTrue(f.result.recoveryRequired)
    }

    @Test fun previousIdentityCannotBeMistakenForAResponseToANewProbe() {
        val f = Fixture(LivePolling())
        val old = f.startReading()
        repeat(2) {
            old.emit(GattEvent.Written)
            old.receive("OK\r>")
            f.fireTimer()
        }
        assertEquals(ElmCommand.Identity, old.commands.last())
        f.probe.cancel()
        val link = f.startReading()
        f.fireTimer()
        assertEquals(0, link.writes)
        assertTrue(f.result.recoveryRequired)
        link.receive("ELM327 v2.3\r>")
        f.fireTimer()
        assertEquals(listOf(ElmCommand.EchoOff), link.commands)
    }

    @Test fun rejectedRecoveryWriteCannotClearAnUnresolvedSession() {
        val f = Fixture(LivePolling())
        f.startReading()
        f.probe.cancel()
        val link = f.startReading()
        f.fireTimer()
        link.emit(GattEvent.WriteRejected("Not queued", retryable = true))
        f.fireTimer()
        val next = f.links.last()
        f.ready(next)
        next.receive(">")
        assertTrue(f.timers.isEmpty())
        assertEquals(0, next.writes)
        assertTrue(f.result.recoveryRequired)
    }

    @Test fun interruptedIdentityProbeRemainsLockedAfterProcessRestart() {
        var saved = false to false
        val original = Fixture(LivePolling(), PromptGate { busy, proof -> saved = busy to proof; true })
        original.startReading()
        original.probe.cancel()
        original.startReading()
        original.fireTimer()
        assertEquals(true to true, saved)

        val reopened = Fixture(LivePolling(), PromptGate(saved.first, saved.second))
        val link = reopened.startReading()
        link.receive("OK\r>")
        assertEquals(0, link.writes)
        assertTrue(reopened.result.recoveryRequired)
        assertTrue(reopened.timers.isEmpty())
    }

    @Test fun anotherDisconnectWhileListeningAutomaticallyResubscribes() {
        val f = Fixture(LivePolling(), PromptGate(unresolved = true))
        val first = f.startReading()
        f.fireTimer()
        first.emit(GattEvent.Failed("Bluetooth was turned off", retryable = true))
        assertEquals(ProbeStage.Retrying, f.result.stage)
        assertTrue(first.closed)
        f.fireTimer()
        val second = f.links.last()
        f.ready(second)
        assertEquals(ProbeStage.Recovering, f.result.stage)
        assertEquals(0, second.writes)
        f.probe.cancel()
        second.receive(">")
        assertTrue(second.closed)
        assertEquals(2, f.links.size)
        assertTrue(f.timers.isEmpty())
    }

    @Test fun liveTimeoutWaitsOnTheSameLinkAndRequiresBothPromptAndWriteCompletion() {
        val f = Fixture(LivePolling())
        val link = f.startReading()
        link.receive("OK\r")
        f.fireTimer()
        assertEquals(ProbeStage.Recovering, f.result.stage)
        assertFalse(link.closed)
        link.receive(">")
        assertEquals(1, link.writes)
        assertEquals(ProbeStage.Recovering, f.result.stage)
        link.emit(GattEvent.Written)
        f.fireTimer()
        assertEquals(listOf(ElmCommand.EchoOff, ElmCommand.EchoOff), link.commands)
        assertEquals(1, f.links.size)
    }

    @Test fun liveAdapterSearchGoesFromABriefBurstToSparseRetriesAndCancels() {
        val f = Fixture(LivePolling())
        f.start()
        val delays = mutableListOf<Long>()
        repeat(4) {
            f.fireTimer()
            assertEquals(ProbeStage.Retrying, f.result.stage)
            assertTrue(f.links.last().closed)
            delays.add(f.nextDelay)
            if (it < 3) f.fireTimer()
        }
        assertEquals(listOf(1000L, 2000L, 10_000L, 10_000L), delays)
        val late = f.timers.single()
        f.probe.cancel()
        late()
        assertEquals(4, f.links.size)
        assertEquals(ProbeStage.Cancelled, f.result.stage)
    }

    @Test fun androidLinkTimeoutAndUnqueuedDisconnectResumeWithoutBlindWriting() {
        val f = Fixture(LivePolling())
        val old = f.start()
        old.emit(GattEvent.Found)
        old.emit(GattEvent.Failed("Link timeout", retryableGattError(BluetoothGatt.GATT_CONNECTION_TIMEOUT)))
        assertEquals(ProbeStage.Retrying, f.result.stage)
        assertTrue(old.closed)
        f.fireTimer()
        val next = f.links.last()
        f.ready(next)
        assertEquals(ElmCommand.EchoOff, next.commands.last())
        next.emit(GattEvent.WriteRejected("Not connected", retryableGattWrite(4)))
        assertEquals(ProbeStage.Retrying, f.result.stage)
        f.fireTimer()
        val third = f.links.last()
        f.ready(third)
        assertEquals(listOf(ElmCommand.EchoOff), third.commands)
        third.emit(GattEvent.WriteRejected("Not allowed", retryableGattWrite(BluetoothStatusCodes.ERROR_GATT_WRITE_NOT_ALLOWED)))
        assertEquals(ProbeStage.Failed, f.result.stage)
        assertFalse(f.result.recoveryRequired)
        assertTrue(retryableGattError(BluetoothGatt.GATT_FAILURE))
        assertTrue(retryableGattWrite(BluetoothStatusCodes.ERROR_BLUETOOTH_NOT_ENABLED))
        assertFalse(retryableGattError(BluetoothGatt.GATT_INSUFFICIENT_AUTHENTICATION))
    }

    @Test fun unexpectedBytesBetweenTransactionsRetireTheLink() {
        val f = Fixture(LivePolling())
        val link = f.startReading()
        link.emit(GattEvent.Written)
        link.receive("OK\r>")
        val nextWrite = f.timers.single()
        link.receive("ERROR\r>")
        nextWrite()
        assertTrue(link.closed)
        assertEquals(1, link.writes)
        assertEquals(ProbeStage.Failed, f.result.stage)
    }

    @Test fun recoveryAcceptsPromptBeforeCccdCallbackButWaitsForSubscriptionSuccess() {
        val f = Fixture(LivePolling())
        val old = f.startReading()
        f.probe.cancel()
        assertTrue(old.closed)
        val link = f.start()
        link.emit(GattEvent.Found)
        link.emit(GattEvent.Connected)
        link.emit(GattEvent.Discovered)
        link.receive("OK\r>")
        assertEquals(0, link.writes)
        link.emit(GattEvent.Subscribed)
        f.fireTimer()
        assertEquals(listOf(ElmCommand.EchoOff), link.commands)
    }

    @Test fun unexpectedBytesAfterTheRecoveryPromptBlockInitialization() {
        val f = Fixture(LivePolling())
        f.startReading()
        f.probe.cancel()
        val link = f.startReading()
        link.receive("OK\r>")
        val nextWrite = f.timers.single()
        link.receive("ERROR\r>")
        nextWrite()
        assertTrue(link.closed)
        assertEquals(0, link.writes)
    }

    @Test fun aLateWriteCallbackDoesNotRefreshAnEarlierPhysicalResponse() {
        val polling = LivePolling()
        val f = Fixture(polling)
        val link = f.startReading()
        for (response in listOf("OK", "OK", "ELM327 v2.3", "OK", "410000100000", "A5")) {
            link.emit(GattEvent.Written)
            link.receive("$response\r>")
            f.fireTimer()
        }
        val at = Instant.parse("2026-11-23T12:00:00Z")
        link.emit(GattEvent.Bytes("410C0DF2\r>".toByteArray(), Receipt(0, at)))
        link.emit(GattEvent.Written)
        val json = JSONObject(polling.json(f.result, 3000, at.plusMillis(3000)))
        assertTrue(json.isNull("rpm"))
        assertEquals(at.toString(), json.getJSONObject("signals").getJSONObject("rpm").getString("captured_at"))
    }

    @Test fun liveAndTheAdapterCheckCannotOverlapAndReleaseTheLeaseOnCancel() {
        val gate = PromptGate()
        val first = Fixture(LivePolling(), gate)
        val check = Fixture(gate = gate)
        val link = first.start()
        check.probe.start()
        assertTrue(check.links.isEmpty())
        assertEquals(ProbeStage.Failed, check.result.stage)
        first.probe.cancel()
        assertTrue(link.closed)
        assertTrue(check.start().started)
    }

    @Test fun lostProcessLeavesDurableUncertaintyUntilThePromptOrAnExplicitPowerCycle() {
        var saved = false
        val first = Fixture(LivePolling(), PromptGate { busy, _ -> saved = busy; true })
        first.startReading()
        assertTrue(saved)

        val afterRestart = PromptGate(saved) { busy, _ -> saved = busy; true }
        val second = Fixture(LivePolling(), afterRestart)
        val link = second.startReading()
        assertEquals(ProbeStage.Recovering, second.result.stage)
        assertEquals(0, link.writes)
        assertFalse(afterRestart.confirmPowerCycle())
        second.fireTimer()
        assertTrue(second.result.recoveryRequired)
        assertTrue(saved)

        assertTrue(second.probe.confirmPowerCycleAndRestart())
        assertTrue(link.closed)
        assertTrue(saved)
        assertFalse(afterRestart.unresolved)
        val recovered = second.links.last()
        assertEquals(0, recovered.writes)
        second.ready(recovered)
        assertEquals(listOf(ElmCommand.EchoOff), recovered.commands)
        assertFalse(second.probe.confirmPowerCycleAndRestart())
    }

    @Test fun synchronousWriteRejectionAllowsFreshSetupWithoutAnOldPrompt() {
        val f = Fixture()
        val link = f.startReading()
        link.emit(GattEvent.WriteRejected("Not queued"))
        assertEquals(ProbeResult(ProbeStage.Failed, "Not queued"), f.result)
        assertEquals(listOf(ElmCommand.Identity), f.startReading().commands)
    }

    @Test fun writesOnlyAfterSetupAndWaitsForFragmentedPromptAndWriteCompletion() {
        val f = Fixture()
        val link = f.start()
        link.emit(GattEvent.Found)
        link.emit(GattEvent.Connected)
        link.emit(GattEvent.Discovered)
        assertEquals(0, link.writes)
        link.emit(GattEvent.Subscribed)
        assertEquals(1, link.writes)
        link.receive("ATI\rELM")
        link.receive("327 v2.3\r")
        assertEquals(ProbeStage.Reading, f.result.stage)
        link.receive(">")
        assertEquals(ProbeStage.Reading, f.result.stage)
        link.emit(GattEvent.Written)
        assertEquals(ProbeResult(ProbeStage.Success, "ELM327 v2.3"), f.result)
        assertTrue(link.closed)
    }

    @Test fun requiresAcknowledgedSubscriptionBeforeWriting() {
        val f = Fixture()
        val link = f.start()
        link.emit(GattEvent.Subscribed)
        assertEquals(0, link.writes)
        link.emit(GattEvent.Found)
        link.emit(GattEvent.Connected)
        link.emit(GattEvent.Discovered)
        link.emit(GattEvent.Failed("Missing CCCD"))
        assertEquals(ProbeResult(ProbeStage.Failed, "Missing CCCD"), f.result)
        assertTrue(link.closed)
        assertEquals(1, f.links.size)
    }

    @Test fun closesOldLinkBeforeRetryAndIgnoresItsCallbacks() {
        val f = Fixture()
        val old = f.start()
        old.emit(GattEvent.Found)
        old.emit(GattEvent.Failed("Disconnected", retryable = true))
        assertTrue(old.closed)
        assertEquals(ProbeStage.Retrying, f.result.stage)
        f.fireTimer()
        val current = f.links.last()
        old.emit(GattEvent.Found)
        old.emit(GattEvent.Failed("Old link"))
        assertFalse(current.closed)
        assertEquals(ProbeStage.Scanning, f.result.stage)
        current.emit(GattEvent.Found)
        current.emit(GattEvent.Failed("Disconnected again", retryable = true))
        assertEquals(ProbeResult(ProbeStage.Failed, "Disconnected again"), f.result)
        assertTrue(current.closed)
        assertEquals(2, f.links.size)
    }

    @Test fun scanTimeoutAndCancellationLeaveNoActiveLinkOrTimer() {
        val f = Fixture()
        assertTrue(f.start().started)
        f.fireTimer()
        assertEquals(ProbeStage.Failed, f.result.stage)
        assertTrue(f.links.single().closed)
        assertTrue(f.timers.isEmpty())
        val second = f.start()
        f.probe.cancel()
        assertEquals(ProbeStage.Cancelled, f.result.stage)
        second.emit(GattEvent.Found)
        assertTrue(second.closed)
        assertTrue(f.timers.isEmpty())
        assertEquals(ProbeStage.Cancelled, f.result.stage)
    }

    @Test fun cancelDuringRetryDoesNotStartAnotherScan() {
        val f = Fixture()
        val link = f.start()
        link.emit(GattEvent.Found)
        link.emit(GattEvent.Failed("Disconnected", retryable = true))
        val lateTimer = f.timers.single()
        f.probe.cancel()
        lateTimer()
        assertEquals(1, f.links.size)
        assertEquals(ProbeStage.Cancelled, f.result.stage)
    }

    @Test fun aCommandTimeoutClosesTheLinkAndDoesNotWriteAgain() {
        val f = Fixture()
        val link = f.startReading()
        link.emit(GattEvent.Written)
        link.receive("ELM327 v2.3")
        f.fireTimer()
        assertEquals(ProbeStage.Failed, f.result.stage)
        assertEquals(1, link.writes)
        assertTrue(link.closed)
    }

    @Test fun rejectsErrorEmptyAndOversizedFrames() {
        for (response in listOf(
            "ATI\r>", "NO DATA\r>", "DATA ERROR\r>", "FB ERROR\r>", "?\r>",
            "41 0C 0D 43\r>", "ELM327 v2.3\rERROR\r>", "ELM327 v2.3\r>ERROR\r>",
            "x".repeat(4097),
        )) {
            val f = Fixture()
            val link = f.startReading()
            link.emit(GattEvent.Written)
            link.receive(response)
            assertEquals(response.take(32), ProbeStage.Failed, f.result.stage)
            assertTrue(link.closed)
        }
    }

    @Test fun acceptsLineEndingsAfterTheIdentityPrompt() {
        val f = Fixture()
        val link = f.startReading()
        link.emit(GattEvent.Written)
        link.receive("ELM327 v2.3\r>\r\n")
        assertEquals(ProbeResult(ProbeStage.Success, "ELM327 v2.3"), f.result)
    }

    @Test fun aWriteFailureCannotBeMaskedByAnEarlyPrompt() {
        val f = Fixture()
        val link = f.startReading()
        link.receive("ELM327 v2.3\r>")
        link.emit(GattEvent.Failed("Write failed"))
        assertEquals(ProbeResult(ProbeStage.Failed, "Write failed"), f.result)
        assertTrue(link.closed)
    }

    @Test fun aWriteFailureWithoutPromptLeavesTheSessionUnresolved() {
        val f = Fixture()
        val link = f.startReading()
        link.emit(GattEvent.Failed("Write failed"))
        assertEquals(ProbeResult(ProbeStage.Failed, "Write failed", recoveryRequired = true), f.result)
        assertTrue(link.closed)
    }

    private class FakeLink(private val listener: (GattEvent) -> Unit) : GattLink {
        var started = false
        var closed = false
        val commands = mutableListOf<ElmCommand>()
        val writes get() = commands.size
        override fun start() { started = true }
        override fun write(command: ElmCommand) { check(!closed); commands.add(command) }
        override fun close() { closed = true }
        fun emit(event: GattEvent) = listener(event)
        fun receive(value: String) = emit(GattEvent.Bytes(value.toByteArray(Charsets.US_ASCII)))
    }

    private class Fixture(live: LivePolling? = null, gate: PromptGate = PromptGate()) {
        val timers = mutableListOf<() -> Unit>()
        var nextDelay = 0L
        private var tick = 0L
        val links = mutableListOf<FakeLink>()
        var result = ProbeResult(ProbeStage.Idle)
        val probe = AdapterProbe(
            open = { listener ->
                check(links.all { it.closed })
                FakeLink(listener).also(links::add)
            },
            schedule = { delay, callback ->
                nextDelay = delay
                timers.add(callback)
                val cancel: () -> Unit = { timers.remove(callback) }
                cancel
            },
            onResult = { result = it },
            live = live,
            promptGate = gate,
            elapsed = { tick },
        )

        fun start(): FakeLink {
            probe.start()
            return links.last()
        }

        fun fireTimer() {
            tick += nextDelay
            timers.single().invoke()
        }
        fun startReading() = start().also(::ready)

        fun ready(link: FakeLink) = with(link) {
            emit(GattEvent.Found)
            emit(GattEvent.Connected)
            emit(GattEvent.Discovered)
            emit(GattEvent.Subscribed)
        }
    }
}
