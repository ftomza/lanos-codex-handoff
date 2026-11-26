package io.github.ftomza.lanos

import android.annotation.SuppressLint
import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.os.SystemClock
import android.view.View
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.view.WindowManager
import android.webkit.WebMessage
import android.webkit.WebMessagePort
import android.webkit.WebSettings
import android.webkit.WebView
import android.widget.FrameLayout
import android.widget.Toast
import java.io.File
import java.io.IOException
import java.time.Instant
import org.json.JSONObject

class MainActivity : Activity() {
    private lateinit var dashboard: WebView
    private val main = Handler(Looper.getMainLooper())
    private val simulator = Simulator()
    private val live = LivePolling(LiveMil(::recordMil), ::recordGeneration)
    private val bluetooth by lazy { BluetoothAccess(this, main) }
    private val liveProbe by lazy { bluetooth.probe(live, ::acceptLiveResult) }
    private val tripJournal by lazy {
        TripJournal(TripArchive(File(noBackupFilesDir, TRIP_DIRECTORY)), TripJournal.io) {
            main.post {
                if (!isDestroyed) Toast.makeText(this, "Не удалось записать журнал LIVE", Toast.LENGTH_LONG).show()
            }
        }
    }
    private val fuelTrips = mutableMapOf<FuelMode, FuelComputer>()
    private val fuelSettings by lazy { getSharedPreferences("fuel-settings", MODE_PRIVATE) }
    private var allowUnverifiedIat = false
    private val fuelMode: FuelMode
        get() = when {
            !liveSelected -> FuelMode.Simulator
            allowUnverifiedIat -> FuelMode.Experimental
            else -> FuelMode.Live
        }
    private var iatDialog: AlertDialog? = null
    private var powerCycleDialog: AlertDialog? = null
    private var liveResult = ProbeResult(ProbeStage.Cancelled, "Нажмите «Подключить авто», чтобы начать.")
    private var liveSelected = false
    private var permissionPending = false
    private var resumeLive = false
    private val origin = Uri.parse(AssetPolicy.ORIGIN)
    private var startedAt = 0L
    private var resumed = false
    private var loaded = false
    private var port: WebMessagePort? = null
    private var awaitingReady = false
    private val account = object : Runnable {
        override fun run() {
            val tick = SystemClock.elapsedRealtime()
            observeFuel(tick, Instant.now())
            main.postDelayed(this, 250)
        }
    }
    private val publish = Runnable {
        val channel = port ?: return@Runnable
        val tick = SystemClock.elapsedRealtime()
        val now = Instant.now()
        val sample = if (liveSelected) live.snapshot(liveResult, tick, now)
            else simulator.sample(tick - startedAt, now).payload()
        sample.put("fuel", fuelTrip(tick, now).view(tick))
        awaitingReady = true
        channel.postMessage(WebMessage(sample.toString()))
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        startedAt = SystemClock.elapsedRealtime()
        allowUnverifiedIat = fuelSettings.getBoolean("allow-unverified-iat", false)
        liveSelected = savedInstanceState?.getBoolean("liveSelected") == true
        resumeLive = liveSelected && savedInstanceState?.getBoolean("resumeLive") == true
        dashboard = createDashboard()
        val root = FrameLayout(this).apply {
            setBackgroundColor(0xff101815.toInt())
            addView(dashboard, FrameLayout.LayoutParams(-1, -1))
            if (Build.VERSION.SDK_INT >= 30) {
                setOnApplyWindowInsetsListener { view, insets ->
                    val cutout = insets.getInsets(WindowInsets.Type.displayCutout())
                    view.setPadding(cutout.left, cutout.top, cutout.right, cutout.bottom)
                    insets
                }
            }
        }
        setContentView(root)
        enterFullscreen()
        dashboard.loadUrl(AssetPolicy.DASHBOARD)
    }

    @SuppressLint("SetJavaScriptEnabled") // Only the bundled dashboard can execute scripts.
    private fun createDashboard() = WebView(this).apply {
        setBackgroundColor(0xff101815.toInt())
        settings.apply {
            javaScriptEnabled = true
            allowFileAccess = false
            allowContentAccess = false
            blockNetworkLoads = true
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
        }
        webViewClient = DashboardClient(assets,
            onLoading = {
                loaded = false
                closeFeed()
            },
            onLoaded = {
                loaded = true
                openFeed()
            },
            onAction = ::dashboardAction,
        )
    }

    override fun onResume() {
        super.onResume()
        resumed = true
        main.post(account)
        dashboard.onResume()
        enterFullscreen()
        openFeed()
        if (resumeLive) startLive()
    }

    override fun onPause() {
        resumed = false
        main.removeCallbacks(account)
        if (liveSelected) {
            val interactive = getSystemService(PowerManager::class.java).isInteractive
            if (!interactive || !permissionPending) endTrip(if (interactive) "background" else "screen_off")
        }
        suspendFuel("background")
        iatDialog?.dismiss()
        powerCycleDialog?.dismiss()
        if (liveSelected) liveProbe.cancel()
        closeFeed()
        if (loaded) dashboard.postWebMessage(WebMessage("lanos:paused"), origin)
        dashboard.onPause()
        super.onPause()
    }

    override fun onStop() {
        if (permissionPending) endTrip("permission_prompt_interrupted")
        permissionPending = false
        super.onStop()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putBoolean("liveSelected", liveSelected)
        outState.putBoolean("resumeLive", resumeLive)
        super.onSaveInstanceState(outState)
    }

    private fun dashboardAction(action: String) {
        if (!resumed) return
        when (action) {
            AssetPolicy.PROBE -> {
                if (liveSelected) {
                    endTrip("adapter_check")
                    liveProbe.cancel()
                }
                startActivity(Intent(this, ProbeActivity::class.java))
            }
            AssetPolicy.LIVE -> {
                if (!liveSelected) selectSource(live = true)
                startLive(mayRequestPermission = true)
            }
            AssetPolicy.SIMULATOR -> if (liveSelected) {
                endTrip("source_switch")
                liveProbe.cancel()
                selectSource(live = false)
            }
            AssetPolicy.FUEL_IAT -> if (liveSelected) showIatConfirmation()
            AssetPolicy.VGATE_POWER_CYCLE -> showPowerCycleConfirmation()
            AssetPolicy.TRIP_JOURNAL -> {
                if (liveSelected) endTrip("journal_opened")
                startActivity(Intent(this, TripJournalActivity::class.java))
            }
        }
    }

    private fun selectSource(live: Boolean) {
        suspendFuel("source_switch")
        closeFeed()
        liveSelected = live
        if (!live) resumeLive = false
        openFeed()
    }

    private fun fuelTrip(tick: Long, now: Instant): FuelComputer = fuelTrips.getOrPut(fuelMode) {
        FuelComputer(fuelMode, FileFuelStore(noBackupFilesDir, fuelMode), tick, now)
    }

    private fun observeFuel(tick: Long, now: Instant) {
        val elapsed = tick - startedAt
        if (liveSelected) live.mil.expire(tick)
        val inputs = if (liveSelected) live.fuelInputs(liveResult.stage, tick)
            else simulator.sample(elapsed, now).fuelInputs(tick, elapsed / 60_000)
        val computer = fuelTrip(tick, now)
        val before = computer.state
        computer.observe(tick, now, inputs)
        if (!liveSelected || !tripJournal.recording) return
        val after = computer.state
        val covered = if (after.observedMs > before.observedMs) after.gapMs == before.gapMs else null
        tripJournal.observe(tick, now, inputs.generation, covered) {
            tripSample(live, liveResult, tick, now, computer)
        }
    }

    private fun recordGeneration(generation: Long) {
        if (resumed && liveSelected && tripJournal.recording) {
            tripJournal.generation(SystemClock.elapsedRealtime(), Instant.now(), generation)
        }
    }

    private fun recordMil(notice: MilNotice) {
        if (liveSelected && tripJournal.recording) {
            val tick = notice.observedTick ?: SystemClock.elapsedRealtime()
            tripJournal.milNotice(tick, Instant.now(), live.generation, notice)
        }
    }

    private fun beginTrip() {
        if (!tripJournal.recording) live.mil.newSession()
        tripJournal.begin(SystemClock.elapsedRealtime(), Instant.now(), fuelMode, live.generation)
    }

    private fun endTrip(reason: String) {
        live.mil.invalidate(reason)
        tripJournal.end(SystemClock.elapsedRealtime(), Instant.now(), reason)
    }

    private fun suspendFuel(reason: String) {
        fuelTrips[fuelMode]?.suspendTrip(SystemClock.elapsedRealtime(), Instant.now(), reason)
    }

    private fun acceptLiveResult(result: ProbeResult) {
        val previous = liveResult
        liveResult = result
        if (!result.recoveryRequired) powerCycleDialog?.dismiss()
        if (!resumed || !liveSelected) return
        val tick = SystemClock.elapsedRealtime()
        val now = Instant.now()
        if (previous.stage != result.stage || previous.recoveryRequired != result.recoveryRequired) {
            tripJournal.transition(tick, now, result.stage, live.generation, result.recoveryRequired)
        }
        result.identityRecovery?.let { outcome ->
            tripJournal.event(tick, now, "vgate_ati_recovery",
                JSONObject().put("outcome", outcome.name.lowercase()).put("generation", live.generation))
        }
        if (previous.stage == ProbeStage.Live && result.stage != ProbeStage.Live) observeFuel(tick, now)
    }

    private fun showPowerCycleConfirmation() {
        if (!liveSelected || !liveResult.recoveryRequired || powerCycleDialog != null) return
        powerCycleDialog = AlertDialog.Builder(this)
            .setTitle("Питание Vgate")
            .setMessage("Если Vgate всё ещё не отвечает, выньте его из OBD-разъёма на стоянке. Дождитесь погасания индикатора, вставьте адаптер обратно и подтвердите отключение питания.")
            .setNegativeButton("Закрыть", null)
            .setPositiveButton("Питание отключал") { _, _ ->
                if (!resumed || !liveSelected || !liveResult.recoveryRequired) return@setPositiveButton
                if (liveProbe.confirmPowerCycleAndRestart()) {
                    tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "vgate_power_cycle_confirmed")
                }
            }
            .setOnDismissListener { powerCycleDialog = null }
            .show()
    }

    private fun showIatConfirmation() {
        if (iatDialog != null) return
        val enable = !allowUnverifiedIat
        iatDialog = AlertDialog.Builder(this)
            .setTitle("Оценка расхода LIVE")
            .setMessage(if (enable)
                "Включить экспериментальную оценку по IAT? Показания датчика и точность расчёта не проверены. Эта поездка хранится отдельно."
                else "Выключить экспериментальную оценку? Её поездка сохранится отдельно.")
            .setNegativeButton("Отмена", null)
            .setPositiveButton(if (enable) "Включить" else "Выключить") { _, _ ->
                if (resumed && liveSelected) {
                    if (fuelSettings.edit().putBoolean("allow-unverified-iat", enable).commit()) {
                        suspendFuel("configuration_switch")
                        closeFeed()
                        allowUnverifiedIat = enable
                        tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "fuel_configuration",
                            JSONObject().put("fuel_mode", fuelMode.name))
                        openFeed()
                    } else {
                        Toast.makeText(this, "Не удалось сохранить настройку IAT", Toast.LENGTH_SHORT).show()
                    }
                }
            }
            .setOnDismissListener { iatDialog = null }
            .show()
    }

    private fun handleFuelControl(channel: WebMessagePort, data: String) {
        val request = FuelControlRequest.decode(data) ?: return
        val tick = SystemClock.elapsedRealtime()
        val now = Instant.now()
        // Check current coverage before changing state, even if a WebView
        // request arrives between two scheduled accounting ticks.
        observeFuel(tick, now)
        val reply = JSONObject().put("type", "fuel_reply").put("id", request.id)
        try {
            val computer = fuelTrip(tick, now)
            reply.put("result", request.execute(computer, tick, now))
            if (liveSelected && request.action !is FuelAction.Preview) {
                val event = if (request.action is FuelAction.Reset) "fuel_reset" else "fuel_calibration"
                tripJournal.event(tick, now, event, JSONObject().put("revision", computer.state.revision)
                    .put("factor", computer.state.factor))
            }
        } catch (e: IOException) {
            reply.put("error", e.message)
        } catch (e: IllegalArgumentException) {
            reply.put("error", e.message ?: "Проверьте параметры поездки.")
        }
        channel.postMessage(WebMessage(reply.toString()))
    }

    private fun startLive(mayRequestPermission: Boolean = false) {
        if (!resumed || !liveSelected || permissionPending || liveProbe.running) return
        beginTrip()
        val unavailable = bluetooth.unavailable()
        if (unavailable != null) {
            acceptLiveResult(ProbeResult(ProbeStage.Failed, unavailable))
            tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "bluetooth_unavailable")
            return
        }
        val missing = bluetooth.missing()
        if (missing.isEmpty()) {
            resumeLive = true
            liveProbe.start()
        } else if (mayRequestPermission) {
            permissionPending = true
            tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "permission_requested")
            acceptLiveResult(ProbeResult(ProbeStage.Connecting, "Ожидаем разрешение Bluetooth."))
            requestPermissions(missing, LIVE_PERMISSION)
        } else {
            resumeLive = false
            acceptLiveResult(ProbeResult(ProbeStage.Failed, bluetooth.denied()))
            tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "permission_missing")
            endTrip("permission_missing")
        }
    }

    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode != LIVE_PERMISSION || !permissionPending) return
        permissionPending = false
        if (bluetooth.missing().isNotEmpty()) {
            resumeLive = false
            acceptLiveResult(ProbeResult(ProbeStage.Failed, bluetooth.denied()))
            tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "permission_denied")
            endTrip("permission_denied")
            return
        }
        tripJournal.event(SystemClock.elapsedRealtime(), Instant.now(), "permission_granted")
        resumeLive = true
        if (resumed) startLive()
    }

    override fun onDestroy() {
        closeFeed()
        (dashboard.parent as? FrameLayout)?.removeView(dashboard)
        dashboard.destroy()
        super.onDestroy()
    }

    private fun openFeed() {
        if (!resumed || !loaded || port != null) return
        val (native, page) = dashboard.createWebMessageChannel()
        port = native
        awaitingReady = true
        native.setWebMessageCallback(object : WebMessagePort.WebMessageCallback() {
            override fun onMessage(channel: WebMessagePort, message: WebMessage) {
                if (channel !== port || !resumed) return
                val data = message.data ?: return
                if (data != "ready") {
                    handleFuelControl(channel, data)
                    return
                }
                if (!awaitingReady) return
                awaitingReady = false
                // At most one unacknowledged snapshot is in flight, even if WebView stalls.
                main.postDelayed(publish, 250)
            }
        })
        val channel = if (liveSelected) "lanos:telemetry:live" else "lanos:telemetry"
        dashboard.postWebMessage(WebMessage(channel, arrayOf(page)), origin)
    }

    private fun closeFeed() {
        main.removeCallbacks(publish)
        val previous = port
        port = null
        awaitingReady = false
        previous?.close()
    }

    @Suppress("DEPRECATION")
    private fun enterFullscreen() {
        if (Build.VERSION.SDK_INT >= 30) {
            window.insetsController?.apply {
                systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                hide(WindowInsets.Type.systemBars())
            }
        } else {
            window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_FULLSCREEN or
                View.SYSTEM_UI_FLAG_HIDE_NAVIGATION or View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
        }
    }

    private companion object {
        const val LIVE_PERMISSION = 2
    }
}
