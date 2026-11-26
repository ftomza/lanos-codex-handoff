package io.github.ftomza.lanos

import java.time.Instant

internal enum class ProbeStage {
    Idle, Scanning, Connecting, Discovering, Subscribing, Reading, Recovering,
    Retrying, WaitingEcu, Live, Success, Failed, Cancelled,
}

internal data class ProbeResult(
    val stage: ProbeStage,
    val detail: String = "",
    val recoveryRequired: Boolean = false,
    val identityRecovery: IdentityRecoveryOutcome? = null,
)

internal enum class IdentityRecoveryOutcome { Attempted, Confirmed, Unconfirmed }
internal data class Receipt(val tick: Long, val time: Instant)

internal sealed interface GattEvent {
    data object Found : GattEvent
    data object Connected : GattEvent
    data object Discovered : GattEvent
    data object Subscribed : GattEvent
    data object Written : GattEvent
    data class WriteRejected(val detail: String, val retryable: Boolean = false) : GattEvent
    data class Bytes(val data: ByteArray, val receipt: Receipt? = null) : GattEvent
    data class Failed(val detail: String, val retryable: Boolean = false) : GattEvent
}

internal interface GattLink {
    fun start()
    fun write(command: ElmCommand)
    fun close()
}

// Shared by the two foreground Android entry points. A new GATT connection
// alone does not prove that a command accepted by the old connection ended.
internal class PromptGate(
    unresolved: Boolean = false,
    identityProofRequired: Boolean = false,
    private val save: (busy: Boolean, needsIdentityProof: Boolean) -> Boolean = { _, _ -> true },
) {
    private var inUse = false
    private var outstanding: ElmCommand? = null
    var unresolved = unresolved || identityProofRequired
        private set
    var identityProofRequired = identityProofRequired
        private set
    val canProbeIdentity get() =
        unresolved && !identityProofRequired && outstanding != null && outstanding != ElmCommand.Identity

    fun commandSent(command: ElmCommand) {
        unresolved = true
        outstanding = command
    }

    fun markUnknown() {
        unresolved = true
        outstanding = null
    }

    fun promptReceived() {
        unresolved = false
        outstanding = null
    }

    fun beginIdentityProbe(): Boolean {
        if (!save(true, true)) return false
        commandSent(ElmCommand.Identity)
        identityProofRequired = true
        return true
    }

    fun confirmIdentityProbe(): Boolean {
        if (!save(true, false)) return false
        identityProofRequired = false
        promptReceived()
        return true
    }

    fun acquire(): String? {
        if (inUse) return "Проверка Vgate уже запущена в другом окне."
        // Persist once per session, so a process exit is always conservative.
        if (!save(true, identityProofRequired)) return "Не удалось сохранить состояние сеанса Vgate."
        inUse = true
        return null
    }

    fun release() {
        if (!unresolved && !save(false, false)) markUnknown()
        inUse = false
    }

    fun confirmPowerCycle(): Boolean {
        if (inUse || !save(false, false)) return false
        identityProofRequired = false
        promptReceived()
        return true
    }
}

// The Activity, scheduler and link deliver all calls on the main thread.
internal class AdapterProbe(
    private val open: ((GattEvent) -> Unit) -> GattLink,
    private val schedule: (Long, () -> Unit) -> (() -> Unit),
    private val onResult: (ProbeResult) -> Unit,
    private val live: LivePolling? = null,
    private val promptGate: PromptGate = PromptGate(),
    private val elapsed: () -> Long = { System.nanoTime() / 1_000_000 },
    private val wallTime: () -> Instant = Instant::now,
) {
    private var stage = ProbeStage.Idle
    private var attempt = 0
    private var link: GattLink? = null
    private var session = 0L
    private var cancelTimer: (() -> Unit)? = null
    private var timerId = 0L
    private var pending: ElmCommand? = null
    private var writeCompleted = false
    private var response = ElmFrame()
    private var receivedAt: Receipt? = null
    private var recoveringCurrentLink = false
    private var identityRecovery: ElmIdentityRecovery? = null
    private var offerPowerCycle = false
    private var goodSince: Long? = null
    private val liveSchedule = live?.let(::LiveSchedule)

    val running: Boolean
        get() = link != null || stage == ProbeStage.Retrying

    fun start() {
        if (running) return
        val unavailable = promptGate.acquire()
        if (unavailable != null) {
            report(ProbeStage.Failed, unavailable)
            return
        }
        attempt = 0
        offerPowerCycle = false
        startAttempt()
    }

    fun cancel() {
        if (!running) return
        closeLink()
        promptGate.release()
        offerPowerCycle = false
        report(ProbeStage.Cancelled, "Связь с адаптером остановлена.")
    }

    fun confirmPowerCycleAndRestart(): Boolean {
        if (live == null || !offerPowerCycle || !promptGate.unresolved) return false
        if (running) {
            closeLink()
            promptGate.release()
        }
        if (!promptGate.confirmPowerCycle()) {
            report(ProbeStage.Failed, "Не удалось сохранить подтверждение отключения питания Vgate.")
            return false
        }
        start()
        return true
    }

    private fun startAttempt() {
        val current = ++session
        live?.reset()
        recoveringCurrentLink = false
        goodSince = null
        response = ElmFrame()
        link = open { event ->
            if (session == current) handle(event)
        }
        waitIn(ProbeStage.Scanning, 10_000, "IOS-Vlink не найден. Проверьте питание адаптера и закройте другие OBD-приложения.",
            "Ищем IOS-Vlink…", retryable = live != null)
        link?.start()
    }

    private fun handle(event: GattEvent) {
        when (event) {
            GattEvent.Found -> if (stage == ProbeStage.Scanning) {
                waitIn(ProbeStage.Connecting, 12_000, "Истекло время подключения к IOS-Vlink.",
                    "Подключаемся к IOS-Vlink…", retryable = true)
            }
            GattEvent.Connected -> if (stage == ProbeStage.Connecting) {
                waitIn(ProbeStage.Discovering, 6_000, "Истекло время поиска сервиса BLE.",
                    "Ищем сервис BLE…", retryable = true)
            }
            GattEvent.Discovered -> if (stage == ProbeStage.Discovering) {
                waitIn(ProbeStage.Subscribing, 5_000, "Адаптер не подтвердил подписку BLE.",
                    "Подписываемся на ответы IOS-Vlink…", retryable = true)
            }
            GattEvent.Subscribed -> if (stage == ProbeStage.Subscribing) {
                if (promptGate.unresolved) {
                    if (response.complete) finishRecovery()
                    else waitForPreviousPrompt()
                } else {
                    startCommands()
                }
            }
            GattEvent.Written -> when {
                identityRecovery != null -> {
                    writeCompleted = true
                    finishIdentityRecovery()
                }
                recoveringCurrentLink -> {
                    writeCompleted = true
                    if (response.complete) finishRecovery()
                }
                pending != null -> {
                    writeCompleted = true
                    finishResponse()
                }
            }
            is GattEvent.WriteRejected -> when {
                identityRecovery != null -> fail(event.detail, event.retryable)
                pending != null -> {
                    promptGate.promptReceived()
                    fail(event.detail, event.retryable)
                }
            }
            is GattEvent.Bytes -> receive(event)
            is GattEvent.Failed -> fail(event.detail, event.retryable)
        }
    }

    private fun waitForPreviousPrompt() {
        if (promptGate.identityProofRequired) {
            if (live != null) offerManualRecovery(INTERRUPTED_IDENTITY_PROBE)
            else fail(INTERRUPTED_IDENTITY_PROBE)
            return
        }
        val detail = "Ждём окончания предыдущей команды…"
        if (live == null) {
            waitIn(ProbeStage.Recovering, 8_000, SYNC_TIMEOUT, detail)
            return
        }
        report(ProbeStage.Recovering, if (offerPowerCycle) LIVE_SYNC_WAIT else detail)
        timer(8_000) {
            if (promptGate.canProbeIdentity) probeIdentity()
            else offerManualRecovery(LIVE_SYNC_WAIT)
        }
    }

    private fun probeIdentity() {
        val current = link ?: return
        // A previous ATI could return the same version line. The gate allows
        // this probe only for a known different command, once per interruption.
        if (!promptGate.beginIdentityProbe()) {
            offerManualRecovery("Не удалось сохранить состояние проверки Vgate. $LIVE_SYNC_WAIT")
            return
        }
        identityRecovery = ElmIdentityRecovery()
        writeCompleted = false
        report(ProbeStage.Recovering, "Проверяем ответ Vgate на ATI…", IdentityRecoveryOutcome.Attempted)
        timer(5_000) {
            offerManualRecovery(
                "Vgate не подтвердил синхронизацию по ATI. Приборка продолжает ждать ответ.",
                IdentityRecoveryOutcome.Unconfirmed,
            )
        }
        current.write(ElmCommand.Identity)
    }

    private fun offerManualRecovery(detail: String, outcome: IdentityRecoveryOutcome? = null) {
        stopTimer()
        offerPowerCycle = true
        report(ProbeStage.Recovering, detail, outcome)
    }

    private fun finishIdentityRecovery() {
        val frame = identityRecovery?.frame ?: return
        if (!writeCompleted || !frame.complete) return
        if (!promptGate.confirmIdentityProbe()) {
            fail("Не удалось сохранить подтверждение синхронизации Vgate.")
            return
        }
        response = frame
        identityRecovery = null
        finishRecovery(IdentityRecoveryOutcome.Confirmed)
    }

    private fun receive(event: GattEvent.Bytes) {
        val probe = identityRecovery
        if (probe != null) {
            if (!probe.append(event.data)) fail("Недопустимый ответ Vgate при проверке ATI.")
            else finishIdentityRecovery()
            return
        }
        if (promptGate.identityProofRequired) return
        val earlyRecovery = stage == ProbeStage.Subscribing && promptGate.unresolved
        val recovering = stage == ProbeStage.Recovering
        val trailing = pending == null && response.complete &&
            (stage == ProbeStage.Reading || stage == ProbeStage.Live || stage == ProbeStage.WaitingEcu)
        if (pending == null && !recovering && !earlyRecovery && !trailing) return
        if (!response.append(event.data)) {
            promptGate.markUnknown()
            fail("Ответ адаптера больше 4096 байт или содержит недопустимые данные.")
            return
        }
        if (!response.complete || earlyRecovery || trailing) return
        promptGate.promptReceived()
        if (recovering) {
            if (!recoveringCurrentLink || writeCompleted) finishRecovery()
            return
        }
        if (receivedAt == null) receivedAt = event.receipt ?: Receipt(elapsed(), wallTime())
        finishResponse()
    }

    private fun finishRecovery(outcome: IdentityRecoveryOutcome? = null) {
        recoveringCurrentLink = false
        offerPowerCycle = false
        promptGate.promptReceived()
        report(ProbeStage.Reading, "Связь синхронизирована. Инициализируем адаптер…", outcome)
        timer(250, ::startCommands)
    }

    private fun startCommands() {
        send(liveSchedule?.begin(elapsed())?.command ?: ElmCommand.Identity)
    }

    private fun send(command: ElmCommand) {
        val current = link ?: return
        pending = command
        response = ElmFrame()
        receivedAt = null
        writeCompleted = false
        if (stage != ProbeStage.Live && stage != ProbeStage.WaitingEcu) {
            report(ProbeStage.Reading, "Ожидаем ответ на ${command.wire}…")
        }
        val timeout = when (command) {
            ElmCommand.Capabilities -> 30_000L
            ElmCommand.StoredDtc, ElmCommand.PendingDtc, ElmCommand.MilStatus -> 2000L
            else -> 5000L
        }
        timer(timeout) { commandTimedOut(command) }
        promptGate.commandSent(command)
        current.write(command)
    }

    private fun commandTimedOut(command: ElmCommand) {
        val detail = "Не получен полный ответ на ${command.wire}."
        if (live == null) {
            fail("$detail $SYNC_TIMEOUT")
            return
        }
        pending = null
        recoveringCurrentLink = true
        goodSince = null
        live.mil.invalidate("command_timeout")
        live.reset()
        waitIn(ProbeStage.Recovering, 8_000, "$detail $SYNC_TIMEOUT",
            "$detail Ждём окончания команды…", retryable = true)
    }

    private fun finishResponse() {
        val command = pending ?: return
        if (!writeCompleted || !response.complete) return
        pending = null
        stopTimer()
        if (live == null) {
            val identity = response.identity()
            val preview = response.preview(command)
            closeLink()
            promptGate.release()
            if (identity == null) report(ProbeStage.Failed, "Ожидали строку версии ELM327.\n$preview")
            else report(ProbeStage.Success, identity)
            return
        }
        val receipt = checkNotNull(receivedAt)
        val now = elapsed()
        if (!live.reply(command, response, receipt.tick, receipt.time, now)) {
            fail("Адаптер отклонил ${command.wire}. ${response.preview(command)}")
            return
        }
        val scheduler = checkNotNull(liveSchedule)
        // Scheduling uses command completion, while freshness uses receipt.
        val next = scheduler.completed(command, now)
        when {
            scheduler.waitingForEcu -> {
                goodSince = null
                report(ProbeStage.WaitingEcu, "ЭБУ не отвечает. Если зажигание было выключено, включите его. Приборка сама повторит проверку.")
            }
            scheduler.phase == LiveSchedule.Phase.Poll -> {
                report(ProbeStage.Live)
                if (command.pid != null && command != ElmCommand.Capabilities && live.succeeded(command)) {
                    val since = goodSince ?: now.also { goodSince = it }
                    if (now - since >= 10_000L) attempt = 0
                }
            }
        }
        timer(next.delayMs) { send(next.command) }
    }

    private fun fail(detail: String, retryable: Boolean = false) {
        val outcome = if (identityRecovery != null && !offerPowerCycle) IdentityRecoveryOutcome.Unconfirmed else null
        closeLink()
        if (retryable && live != null) {
            val delay = when (attempt) {
                0 -> 1000L
                1 -> 2000L
                else -> 10_000L
            }
            attempt = minOf(attempt + 1, 2)
            report(ProbeStage.Retrying, "$detail Следующая попытка через ${delay / 1000} с.", outcome)
            timer(delay, ::startAttempt)
        } else if (retryable && attempt == 0) {
            attempt++
            report(ProbeStage.Retrying, detail)
            timer(800, ::startAttempt)
        } else {
            promptGate.release()
            offerPowerCycle = promptGate.unresolved
            report(ProbeStage.Failed, detail, outcome)
        }
    }

    private fun waitIn(next: ProbeStage, delay: Long, timeout: String, detail: String, retryable: Boolean = false) {
        report(next, detail)
        timer(delay) { fail(timeout, retryable) }
    }

    private fun timer(delay: Long, run: () -> Unit) {
        stopTimer()
        val current = timerId
        cancelTimer = schedule(delay) { if (timerId == current) run() }
    }

    private fun stopTimer() {
        timerId++
        cancelTimer?.invoke()
        cancelTimer = null
    }

    private fun closeLink() {
        session++
        pending = null
        recoveringCurrentLink = false
        identityRecovery = null
        stopTimer()
        val old = link
        link = null
        old?.close()
        live?.mil?.invalidate("link_closed")
    }

    private fun report(next: ProbeStage, detail: String = "", outcome: IdentityRecoveryOutcome? = null) {
        stage = next
        onResult(ProbeResult(next, detail, recoveryRequired = offerPowerCycle && promptGate.unresolved, identityRecovery = outcome))
    }

    private companion object {
        const val LIVE_SYNC_WAIT = "Vgate не завершил предыдущую команду. Приборка продолжает ждать ответ."
        const val INTERRUPTED_IDENTITY_PROBE = "Проверка ATI прервалась. Отключите питание Vgate на стоянке."
        const val SYNC_TIMEOUT = "Нажмите кнопку подключения, чтобы повторить ожидание или подтвердить отключение питания Vgate."
    }
}
