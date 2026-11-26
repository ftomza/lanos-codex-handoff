package io.github.ftomza.lanos

import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.nio.file.Files
import java.nio.file.NoSuchFileException
import java.nio.file.StandardCopyOption

internal class FileFuelStore(directory: File, mode: FuelMode) : FuelStore {
    private val file = File(directory, when (mode) {
        FuelMode.Simulator -> "fuel-simulator.json"
        FuelMode.Live -> "fuel-live.json"
        FuelMode.Experimental -> "fuel-live-experimental.json"
    })

    override fun load(): FuelState? = try {
        Files.newInputStream(file.toPath()).buffered().use { input ->
            val bytes = ByteArray(8192)
            var size = 0
            while (size < bytes.size) {
                val count = input.read(bytes, size, bytes.size - size)
                if (count < 0) break
                size += count
            }
            if (size == bytes.size) throw IOException("Файл поездки слишком большой")
            FuelState.parse(String(bytes, 0, size, Charsets.UTF_8))
        }
    } catch (_: NoSuchFileException) {
        null
    }

    override fun save(state: FuelState) {
        val target = file.toPath()
        val temporary = Files.createTempFile(target.parent, file.name, ".tmp")
        try {
            FileOutputStream(temporary.toFile()).use { out ->
                out.write(state.json().toString().toByteArray(Charsets.UTF_8))
                out.fd.sync()
            }
            Files.move(temporary, target, StandardCopyOption.ATOMIC_MOVE, StandardCopyOption.REPLACE_EXISTING)
        } catch (e: Exception) {
            try {
                Files.deleteIfExists(temporary)
            } catch (cleanup: IOException) {
                e.addSuppressed(cleanup)
            }
            throw e
        }
    }
}
