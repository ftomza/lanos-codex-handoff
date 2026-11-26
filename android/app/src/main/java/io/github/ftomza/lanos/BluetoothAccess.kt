package io.github.ftomza.lanos

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.bluetooth.BluetoothManager
import android.content.Context
import android.content.pm.PackageManager
import android.location.LocationManager
import android.os.Build
import android.os.Handler
import android.os.SystemClock

internal class BluetoothAccess(private val context: Context, private val main: Handler) {
    private val permissions: Array<String>
        get() = if (Build.VERSION.SDK_INT >= 31) {
            arrayOf(Manifest.permission.BLUETOOTH_SCAN, Manifest.permission.BLUETOOTH_CONNECT)
        } else {
            arrayOf(Manifest.permission.ACCESS_FINE_LOCATION)
        }

    private val adapter get() = context.getSystemService(BluetoothManager::class.java)?.adapter
    private val gate: PromptGate
        get() {
            sharedGate?.let { return it }
            val preferences = context.applicationContext.getSharedPreferences("vgate_session", Context.MODE_PRIVATE)
            return PromptGate(
                preferences.getBoolean("may_be_busy", false),
                preferences.getBoolean("identity_proof_required", false),
            ) { uncertain, identityProofRequired ->
                preferences.edit().putBoolean("may_be_busy", uncertain)
                    .putBoolean("identity_proof_required", identityProofRequired).commit()
            }.also { sharedGate = it }
        }

    fun missing(): Array<String> = permissions.filter {
        context.checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED
    }.toTypedArray()

    fun unavailable(): String? = when {
        !context.packageManager.hasSystemFeature(PackageManager.FEATURE_BLUETOOTH_LE) || adapter == null ->
            "На этом устройстве недоступен Bluetooth LE."
        Build.VERSION.SDK_INT < 31 && !legacyLocationEnabled() ->
            "Для поиска BLE на этой версии Android включите геолокацию."
        else -> null
    }

    fun denied(): String {
        val access = if (Build.VERSION.SDK_INT >= 31) "Bluetooth" else "доступ к местоположению"
        return "Разрешите $access в настройках приложения и повторите."
    }

    fun probe(live: LivePolling? = null, onResult: (ProbeResult) -> Unit) = AdapterProbe(
        open = { listener -> AndroidGattLink(context.applicationContext, checkNotNull(adapter), main, listener) },
        schedule = { delay, callback ->
            val pending = Runnable(callback)
            main.postDelayed(pending, delay)
            val cancel: () -> Unit = { main.removeCallbacks(pending) }
            cancel
        },
        onResult = onResult,
        live = live,
        promptGate = gate,
        elapsed = SystemClock::elapsedRealtime,
    )

    fun showRecovery(activity: Activity, completed: (Boolean) -> Unit) {
        AlertDialog.Builder(activity)
            .setTitle("Восстановление связи с Vgate")
            .setMessage("Если питание не отключали, можно ещё раз дождаться ответа. Чтобы начать новый сеанс без ожидания, выньте адаптер из OBD-разъёма, дождитесь, когда погаснет индикатор, и подключите его снова.")
            .setNegativeButton("Ещё раз ждать") { _, _ -> completed(true) }
            .setPositiveButton("Питание отключал") { _, _ -> completed(gate.confirmPowerCycle()) }
            .show()
    }

    private fun legacyLocationEnabled(): Boolean {
        val location = context.getSystemService(LocationManager::class.java) ?: return false
        if (Build.VERSION.SDK_INT >= 28) return location.isLocationEnabled
        return location.isProviderEnabled(LocationManager.GPS_PROVIDER) ||
            location.isProviderEnabled(LocationManager.NETWORK_PROVIDER)
    }

    private companion object {
        var sharedGate: PromptGate? = null
    }
}
