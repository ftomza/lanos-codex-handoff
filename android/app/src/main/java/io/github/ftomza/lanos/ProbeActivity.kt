package io.github.ftomza.lanos

import android.app.Activity
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

class ProbeActivity : Activity() {
    private val main = Handler(Looper.getMainLooper())
    private lateinit var action: Button
    private lateinit var status: TextView
    private lateinit var probe: AdapterProbe
    private var foreground = false
    private var requested = false
    private var recoveryRequired = false

    private val bluetooth by lazy { BluetoothAccess(this, main) }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        probe = bluetooth.probe(onResult = ::render)
        val contents = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(24), dp(20), dp(24), dp(20))
            addView(TextView(context).apply {
                text = "Проверка Vgate"
                textSize = 26f
            })
            addView(TextView(context).apply {
                text = "IOS-Vlink · Bluetooth LE · ATI"
                textSize = 16f
                setPadding(0, dp(8), 0, dp(12))
            })
            status = TextView(context).apply {
                textSize = 18f
                setTextIsSelectable(true)
                accessibilityLiveRegion = View.ACCESSIBILITY_LIVE_REGION_POLITE
                setPadding(0, 0, 0, dp(12))
            }
            addView(status)
            action = Button(context).apply {
                setOnClickListener {
                    when {
                        probe.running -> probe.cancel()
                        recoveryRequired -> bluetooth.showRecovery(this@ProbeActivity) { ready ->
                            if (ready) begin()
                            else render(ProbeResult(ProbeStage.Failed, "Не удалось сбросить состояние Vgate.", recoveryRequired = true))
                        }
                        else -> begin()
                    }
                }
            }
            addView(action)
            addView(Button(context).apply {
                text = "К приборке"
                setOnClickListener { finish() }
            })
        }
        setContentView(ScrollView(this).apply {
            fitsSystemWindows = true
            addView(contents)
        })
        render(ProbeResult(ProbeStage.Idle))
    }

    override fun onStart() {
        super.onStart()
        foreground = true
    }

    override fun onStop() {
        foreground = false
        requested = false
        probe.cancel()
        super.onStop()
    }

    private fun begin() {
        if (!foreground || requested) return
        val unavailable = bluetooth.unavailable()
        if (unavailable != null) {
            render(ProbeResult(ProbeStage.Failed, unavailable))
            return
        }
        val missing = bluetooth.missing()
        if (missing.isEmpty()) {
            probe.start()
            return
        }
        requested = true
        requestPermissions(missing, REQUEST_BLUETOOTH)
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != REQUEST_BLUETOOTH || !requested) return
        requested = false
        if (!foreground) return
        if (bluetooth.missing().isEmpty()) begin()
        else render(ProbeResult(ProbeStage.Failed, bluetooth.denied()))
    }

    private fun render(result: ProbeResult) {
        recoveryRequired = result.recoveryRequired
        status.text = when (result.stage) {
            ProbeStage.Idle -> "Включите Bluetooth. Подайте питание на адаптер и закройте другие OBD-приложения."
            ProbeStage.Scanning -> "Ищем IOS-Vlink…"
            ProbeStage.Connecting -> "Подключаемся к IOS-Vlink…"
            ProbeStage.Discovering -> "Ищем сервис и каналы адаптера…"
            ProbeStage.Subscribing -> "Подписываемся на ответы…"
            ProbeStage.Reading, ProbeStage.Recovering -> result.detail
            ProbeStage.Live -> "Получаем показания…"
            ProbeStage.WaitingEcu -> result.detail
            ProbeStage.Retrying -> "Повторяем соединение. ${result.detail}"
            ProbeStage.Success -> "Ответ ATI от IOS-Vlink\n${result.detail}"
            ProbeStage.Failed -> result.detail
            ProbeStage.Cancelled -> "Проверка остановлена."
        }
        action.text = if (probe.running) "Отменить" else "Проверить адаптер"
    }

    private fun dp(value: Int) = (value * resources.displayMetrics.density).toInt()

    private companion object {
        const val REQUEST_BLUETOOTH = 1
    }
}
