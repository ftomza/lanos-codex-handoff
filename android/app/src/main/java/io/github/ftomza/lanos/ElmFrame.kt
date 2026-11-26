package io.github.ftomza.lanos

internal enum class ElmCommand(val wire: String, val pid: Int? = null) {
    EchoOff("ATE0"),
    LinefeedsOff("ATL0"),
    Identity("ATI"),
    AutoProtocol("ATSP0"),
    Capabilities("0100", 0),
    Protocol("ATDPN"),
    MilStatus("0101", 0x01),
    StoredDtc("03"),
    PendingDtc("07"),
    Voltage("ATRV"),
    Rpm("010C", 0x0c),
    Coolant("0105", 0x05),
    Speed("010D", 0x0d),
    Map("010B", 0x0b),
    Iat("010F", 0x0f),
    ShortTrim("0106", 0x06),
    LongTrim("0107", 0x07),
    ;

    fun bytes(): ByteArray = "${wire}\r".toByteArray(Charsets.US_ASCII)
}

internal class ElmFrame {
    private val text = StringBuilder()
    private var size = 0
    var complete = false
        private set

    fun append(bytes: ByteArray): Boolean = bytes.all(::append)

    fun append(byte: Byte): Boolean {
        if (++size > ELM_RESPONSE_LIMIT) return false
        val char = byte.toInt().toChar()
        if (complete) return char in " \r\n\t"
        if (char == '>') {
            complete = true
        } else {
            if (char !in ' '..'~' && char !in "\r\n\t") return false
            text.append(char)
        }
        return true
    }

    fun lines(command: ElmCommand): List<String> {
        check(complete)
        return text.lineSequence()
            .map { it.trim().replaceFirst(SEARCHING, "").trim() }
            .filter { it.isNotEmpty() && !it.filterNot(Char::isWhitespace).equals(command.wire, ignoreCase = true) }
            .toList()
    }

    fun identity(): String? = lines(ElmCommand.Identity).singleOrNull()?.takeIf(ELM_VERSION::matches)

    fun preview(command: ElmCommand): String =
        lines(command).joinToString(" · ").take(160).ifEmpty { "Ответ пустой." }

    private companion object {
        val SEARCHING = Regex("^SEARCHING\\.{3}", RegexOption.IGNORE_CASE)
        val ELM_VERSION = Regex("ELM327[ \\t]+V?[0-9]+(?:\\.[0-9]+)+[A-Z]?", RegexOption.IGNORE_CASE)
    }
}

// A reconnect can deliver the previous command's tail and the ATI reply
// in one notification. Only a complete identity frame identifies the probe.
internal class ElmIdentityRecovery {
    private var size = 0
    var frame = ElmFrame()
        private set

    fun append(bytes: ByteArray): Boolean {
        size += bytes.size
        if (size > ELM_RESPONSE_LIMIT) return false
        for (byte in bytes) {
            if (!frame.append(byte)) return false
            if (byte == '>'.code.toByte() && frame.identity() == null) frame = ElmFrame()
        }
        return true
    }
}

private const val ELM_RESPONSE_LIMIT = 4096
