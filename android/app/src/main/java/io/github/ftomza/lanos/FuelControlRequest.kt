package io.github.ftomza.lanos

import java.time.Instant
import org.json.JSONObject
import org.json.JSONTokener

internal sealed interface FuelAction {
    data class Reset(val defaultFactor: Boolean) : FuelAction
    data class Preview(val liters: Double, val confirmed: Boolean) : FuelAction
    data class Apply(val liters: Double, val confirmed: Boolean, val token: String) : FuelAction
    data object Undo : FuelAction
    data object Default : FuelAction
}

internal data class FuelControlRequest(val id: Long, val revision: Long, val action: FuelAction) {
    companion object {
        fun decode(data: String): FuelControlRequest? {
            if (data.length > 1024 || !data.startsWith("{")) return null
            return runCatching {
                val reader = JSONTokener(data)
                val request = reader.nextValue() as JSONObject
                while (reader.more()) require(reader.next() in " \n\t\r")
                require(request.fields("type", "id", "operation", "body") && request.get("type") == "fuel")
                val id = request.positiveInteger("id")
                val body = request.getJSONObject("body")
                val revision = body.positiveInteger("expected_revision")
                val action = when (request.get("operation")) {
                    "reset" -> {
                        require(body.fields("expected_revision", "default_factor"))
                        FuelAction.Reset(body.get("default_factor") as Boolean)
                    }
                    "preview" -> {
                        require(body.fields("expected_revision", "measured_liters", "full_to_full"))
                        FuelAction.Preview(body.liters(), body.get("full_to_full") as Boolean)
                    }
                    "calibration" -> calibration(body)
                    else -> error("unknown operation")
                }
                FuelControlRequest(id, revision, action)
            }.getOrNull()
        }

        private fun calibration(body: JSONObject): FuelAction {
            val action = body.get("action")
            if (action == "apply") {
                require(body.fields("action", "expected_revision", "measured_liters", "full_to_full", "preview_token"))
                val token = body.get("preview_token") as String
                require(token.length == 36)
                return FuelAction.Apply(body.liters(), body.get("full_to_full") as Boolean, token)
            }
            require(body.fields("action", "expected_revision"))
            return when (action) {
                "undo" -> FuelAction.Undo
                "default" -> FuelAction.Default
                else -> error("unknown calibration")
            }
        }

        private fun JSONObject.fields(vararg names: String) =
            length() == names.size && names.all(::has)

        private fun JSONObject.positiveInteger(key: String): Long {
            val value = get(key)
            require(value is Int || value is Long)
            return (value as Number).toLong().also { require(it in 1..9_007_199_254_740_991L) }
        }

        private fun JSONObject.liters() = (get("measured_liters") as Number).toDouble()
            .also { require(it.isFinite()) }
    }

    fun execute(computer: FuelComputer, tick: Long, now: Instant): JSONObject {
        when (val command = action) {
            is FuelAction.Preview -> {
                val preview = computer.preview(revision, command.liters, command.confirmed)
                return JSONObject().put("preview_token", preview.token).put("previous_factor", preview.previous)
                    .put("factor", preview.factor).put("adjusted_liters", preview.liters)
            }
            is FuelAction.Reset -> computer.reset(revision, command.defaultFactor, tick, now)
            is FuelAction.Apply -> computer.apply(revision, "apply", command.liters, command.confirmed, command.token, tick, now)
            FuelAction.Undo -> computer.apply(revision, "undo", null, false, null, tick, now)
            FuelAction.Default -> computer.apply(revision, "default", null, false, null, tick, now)
        }
        return JSONObject()
    }
}
