package io.github.ftomza.lanos

internal data class LiveStep(val command: ElmCommand, val delayMs: Long)

// Receives completed, parsed replies only. The caller owns the single GATT
// operation and schedules the returned step with a monotonic clock.
internal class LiveSchedule(private val polling: LivePolling) {
    enum class Phase { Setup, Poll, Standby }

    var phase = Phase.Setup
        private set
    var waitingForEcu = false
        private set

    private val due = mutableMapOf<ElmCommand, Long>()
    private var core = emptySet<ElmCommand>()
    private var priority = emptyList<ElmCommand>()
    private var priorityIndex = 0
    private var supportTurn = false
    private var preferDtc = true
    private var lastSupportWasMil = false
    private val failedCore = mutableSetOf<ElmCommand>()
    private var failedRounds = 0
    private var negativeChecks = 0
    private var ecuDue = 0L
    private var voltageDue = 0L

    fun begin(tick: Long): LiveStep {
        polling.reset()
        resetRounds()
        phase = Phase.Setup
        waitingForEcu = false
        negativeChecks = 0
        ecuDue = tick
        voltageDue = tick
        return LiveStep(initialization.first(), 0L)
    }

    fun completed(command: ElmCommand, tick: Long): LiveStep = when (phase) {
        Phase.Setup -> advanceSetup(command, tick)
        Phase.Poll -> advancePoll(command, tick)
        Phase.Standby -> advanceStandby(command, tick)
    }

    private fun advanceSetup(command: ElmCommand, tick: Long): LiveStep {
        if (command == ElmCommand.Capabilities && !polling.hasBitmap()) return enterStandby(tick)
        if (command == ElmCommand.Protocol) {
            phase = Phase.Poll
            waitingForEcu = false
            resetRounds()
            due.clear()
            for (key in polling.commands()) due[key] = tick
            val supported = due.keys.filter { it.pid != null }
            priority = supported.filter { it in fastSignals }
            priorityIndex = 0
            supportTurn = false
            preferDtc = true
            lastSupportWasMil = false
            core = priority.ifEmpty { supported }.toSet()
            if (core.isEmpty()) due[ElmCommand.Capabilities] = tick + standbyMs
            return nextPoll(tick)
        }
        val next = initialization[initialization.indexOf(command) + 1]
        val delay = if (waitingForEcu && next == ElmCommand.Capabilities) maxOf(gapMs, ecuDue - tick) else gapMs
        return LiveStep(next, delay)
    }

    private fun advancePoll(command: ElmCommand, tick: Long): LiveStep {
        if (command == ElmCommand.Capabilities) {
            if (!polling.hasBitmap()) return enterStandby(tick)
            if (polling.commands().any { it.pid != null }) {
                phase = Phase.Setup
                return LiveStep(ElmCommand.Protocol, gapMs)
            }
            due[command] = tick + standbyMs
            return nextPoll(tick)
        }
        intervals[command]?.let { due[command] = tick + it }
        if (command in core) {
            if (polling.succeeded(command)) resetRounds()
            else {
                failedCore.add(command)
                if (failedCore.containsAll(core)) {
                    failedCore.clear()
                    failedRounds++
                    if (failedRounds >= 3) return enterStandby(tick)
                }
            }
        }
        return nextPoll(tick)
    }

    private fun enterStandby(tick: Long): LiveStep {
        polling.clearEcu()
        phase = Phase.Standby
        waitingForEcu = true
        negativeChecks = 0
        ecuDue = tick + standbyMs
        voltageDue = tick
        return nextStandby(tick)
    }

    private fun advanceStandby(command: ElmCommand, tick: Long): LiveStep {
        if (command == ElmCommand.Voltage) {
            voltageDue = tick + 5000
            return nextStandby(tick)
        }
        check(command == ElmCommand.Capabilities)
        if (polling.hasBitmap()) {
            phase = Phase.Setup
            return LiveStep(ElmCommand.Protocol, gapMs)
        }
        polling.clearEcu()
        ecuDue = tick + standbyMs
        if (++negativeChecks >= 3) {
            phase = Phase.Setup
            return LiveStep(initialization.first(), gapMs)
        }
        return nextStandby(tick)
    }

    private fun nextPoll(tick: Long): LiveStep {
        if (priority.isEmpty()) {
            nextSupport(tick)?.let { return it }
            val (command, nextDue) = due.entries.minBy { it.value }
            return LiveStep(command, maxOf(gapMs, nextDue - tick))
        }
        if (supportTurn) {
            supportTurn = false
            nextSupport(tick)?.let { return it }
        }
        val command = priority[priorityIndex]
        priorityIndex = (priorityIndex + 1) % priority.size
        if (priorityIndex == 0) supportTurn = true
        return LiveStep(command, maxOf(gapMs, due.getValue(command) - tick))
    }

    private fun nextSupport(tick: Long): LiveStep? {
        val readyAt = tick + gapMs
        val milDue = due[ElmCommand.MilStatus]?.let { it <= readyAt } == true
        val regular = due.entries.filter {
            it.key != ElmCommand.MilStatus && it.key !in priority && it.value <= readyAt
        }.minByOrNull { it.value }
        val dtc = polling.mil.nextDtc()
        if (milDue && (!lastSupportWasMil || (dtc == null && regular == null))) {
            lastSupportWasMil = true
            return LiveStep(ElmCommand.MilStatus, gapMs)
        }
        if (dtc != null && (regular == null || preferDtc)) {
            lastSupportWasMil = false
            preferDtc = false
            return LiveStep(dtc, gapMs)
        }
        if (regular == null) return null
        lastSupportWasMil = false
        preferDtc = true
        return LiveStep(regular.key, gapMs)
    }

    private fun nextStandby(tick: Long): LiveStep =
        if (ecuDue <= voltageDue) LiveStep(ElmCommand.Capabilities, maxOf(gapMs, ecuDue - tick))
        else LiveStep(ElmCommand.Voltage, maxOf(gapMs, voltageDue - tick))

    private fun resetRounds() {
        failedCore.clear()
        failedRounds = 0
    }

    private companion object {
        const val gapMs = 150L
        const val standbyMs = 10_000L

        val initialization = listOf(
            ElmCommand.EchoOff, ElmCommand.LinefeedsOff, ElmCommand.Identity,
            ElmCommand.AutoProtocol, ElmCommand.Capabilities, ElmCommand.Protocol,
        )
        val fastSignals = setOf(ElmCommand.Rpm, ElmCommand.Map, ElmCommand.Speed)
        val intervals = mapOf(
            ElmCommand.MilStatus to 5000L,
            ElmCommand.Rpm to 800L,
            ElmCommand.Map to 1000L,
            ElmCommand.Speed to 1000L,
            ElmCommand.Coolant to 3000L,
            ElmCommand.Voltage to 3000L,
            ElmCommand.Iat to 4000L,
            ElmCommand.ShortTrim to 4000L,
            ElmCommand.LongTrim to 12_000L,
        )
    }
}
