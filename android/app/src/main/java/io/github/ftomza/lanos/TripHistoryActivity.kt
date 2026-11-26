package io.github.ftomza.lanos

import android.annotation.SuppressLint
import android.app.Activity
import android.os.Bundle
import android.util.Log
import android.webkit.WebSettings
import android.webkit.WebView
import android.widget.Button
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.TextView
import java.io.File
import java.io.IOException
import java.util.concurrent.Executors
import java.util.concurrent.Future

class TripHistoryActivity : Activity() {
    private var task: Future<*>? = null
    private var chart: WebView? = null
    private lateinit var content: FrameLayout

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        content = FrameLayout(this)
        val status = TextView(this).apply {
            setPadding(22, 12, 22, 12)
            setText(R.string.trip_history_loading)
        }
        content.addView(status)
        setContentView(LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(Button(context).apply {
                setText(R.string.trip_history_back)
                setOnClickListener { finish() }
            }, LinearLayout.LayoutParams(-2, -2))
            addView(content, LinearLayout.LayoutParams(-1, 0, 1f))
            applySystemBarPadding()
        })
        val selected = intent.getStringExtra(SESSION_ID)
        val directory = File(noBackupFilesDir, TRIP_DIRECTORY)
        task = reader.submit {
            val result = runCatching {
                val history = TripHistory(TripArchive(directory))
                val ref = history.catalog().firstOrNull { it.id == selected }
                    ?: throw IOException("Trip unavailable")
                history.report(ref).toString().toByteArray(Charsets.UTF_8)
            }
            runOnUiThread {
                if (isDestroyed || isFinishing) return@runOnUiThread
                result.fold(
                    onSuccess = ::showHistory,
                    onFailure = {
                        Log.w("LanosJournal", "Could not prepare trip report", it)
                        status.setText(R.string.trip_history_error)
                    },
                )
            }
        }
    }

    @SuppressLint("SetJavaScriptEnabled") // Allows the bundled offline chart renderer.
    private fun showHistory(report: ByteArray) {
        val view = WebView(this).apply {
            setBackgroundColor(0xff101815.toInt())
            settings.apply {
                javaScriptEnabled = true
                allowFileAccess = false
                allowContentAccess = false
                blockNetworkLoads = true
                mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            }
            webViewClient = DashboardClient(assets, historyReport = report)
        }
        content.removeAllViews()
        content.addView(view, FrameLayout.LayoutParams(-1, -1))
        chart = view
        view.loadUrl(AssetPolicy.HISTORY)
    }

    override fun onDestroy() {
        task?.cancel(true)
        chart?.let {
            content.removeView(it)
            it.destroy()
        }
        super.onDestroy()
    }

    companion object {
        internal const val SESSION_ID = "session_id"
        private val reader = Executors.newSingleThreadExecutor()
    }
}
