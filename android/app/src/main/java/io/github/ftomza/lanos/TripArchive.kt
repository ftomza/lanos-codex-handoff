package io.github.ftomza.lanos

import java.io.BufferedOutputStream
import java.io.File
import java.io.IOException
import java.io.OutputStream
import java.io.RandomAccessFile
import java.nio.file.Files
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.Locale

internal class TripArchive(
    private val directory: File,
    val maxPartBytes: Int = 2 * 1024 * 1024,
    private val maxParts: Int = 24,
) {
    private fun parts(): List<File> {
        if (!directory.exists()) return emptyList()
        return (directory.listFiles() ?: throw IOException("Could not list journal directory"))
            .filter { filename.matches(it.name) && it.isFile }.sortedByDescending(File::getName)
    }

    fun entries(): List<String> = parts().filter { it.length() > 0 }.map(File::getName)

    fun deletePart(name: String): Boolean {
        require(filename.matches(name)) { "Invalid journal filename" }
        return Files.deleteIfExists(directory.resolve(name).toPath())
    }

    fun openPart(at: Instant): OutputStream {
        if (!directory.isDirectory && !directory.mkdirs()) throw IOException("Could not create journal directory")
        val existing = parts()
        val previous = existing.firstOrNull()?.name?.take(12)?.toLongOrNull() ?: 0L
        if (previous >= 999_999_999_999L) throw IOException("Journal sequence exhausted")
        val sequence = (previous + 1).toString().padStart(12, '0')
        val next = directory.resolve(sequence + "-lanos-live-" + stamp.format(at) + ".jsonl")
        if (!next.createNewFile()) throw IOException("Journal part already exists")
        try {
            for (expired in existing.drop(maxParts - 1)) {
                if (!expired.delete()) throw IOException("Could not remove expired journal part")
            }
            return BufferedOutputStream(next.outputStream())
        } catch (e: IOException) {
            if (!next.delete()) e.addSuppressed(IOException("Could not clean up empty journal part"))
            throw e
        }
    }

    fun visitPart(name: String, visit: (String?) -> Boolean) {
        require(filename.matches(name)) { "Invalid journal filename" }
        RandomAccessFile(directory.resolve(name), "r").use { source ->
            val size = source.length()
            if (size !in 1L..maxPartBytes.toLong()) throw IOException("Invalid journal part length")
            val buffer = ByteArray(8192)
            val line = ByteArray(maxLineBytes)
            var remaining = size
            var used = 0
            while (remaining > 0) {
                if (Thread.currentThread().isInterrupted) throw IOException("Journal read interrupted")
                val count = minOf(remaining, buffer.size.toLong()).toInt()
                source.readFully(buffer, 0, count)
                remaining -= count
                for (index in 0 until count) {
                    val byte = buffer[index]
                    if (byte == '\n'.code.toByte()) {
                        val text = if (used in 1 until line.size) String(line, 0, used, Charsets.UTF_8) else null
                        if (!visit(text)) return
                        used = 0
                    } else if (used < line.size) {
                        line[used++] = byte
                    }
                }
            }
            if (used > 0) visit(null)
        }
    }

    fun export(name: String, output: OutputStream) {
        require(filename.matches(name)) { "Invalid journal filename" }
        RandomAccessFile(directory.resolve(name), "r").use { source ->
            val length = source.length()
            if (length !in 1L..maxPartBytes.toLong()) throw IOException("Invalid journal part length")
            val tail = ByteArray(minOf(length, maxLineBytes.toLong()).toInt())
            source.seek(length - tail.size)
            source.readFully(tail)
            val newline = tail.indexOfLast { it == '\n'.code.toByte() }
            if (newline < 0) throw IOException("No complete record at end of journal part")
            var remaining = length - tail.size + newline + 1
            source.seek(0)
            val buffer = ByteArray(8192)
            while (remaining > 0) {
                val count = minOf(remaining, buffer.size.toLong()).toInt()
                source.readFully(buffer, 0, count)
                output.write(buffer, 0, count)
                remaining -= count
            }
        }
    }

    companion object {
        const val maxLineBytes = 8192
        private val filename = Regex("[0-9]{12}-lanos-live-[0-9]{8}T[0-9]{9}Z\\.jsonl")
        private val stamp = DateTimeFormatter.ofPattern("uuuuMMdd'T'HHmmssSSS'Z'", Locale.ROOT)
            .withZone(ZoneOffset.UTC)
    }
}
