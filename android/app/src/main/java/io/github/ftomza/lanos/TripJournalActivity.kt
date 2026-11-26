package io.github.ftomza.lanos

import android.app.Activity
import android.app.AlertDialog
import android.content.ActivityNotFoundException
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.View
import android.view.WindowInsets
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.TextView
import android.widget.Toast
import java.io.File
import java.io.IOException
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale
import java.util.concurrent.Executors

internal const val TRIP_DIRECTORY = "live-trip-journal"

class TripJournalActivity : Activity() {
    private val archive by lazy { TripArchive(File(noBackupFilesDir, TRIP_DIRECTORY)) }
    private val cleanup by lazy { TripCleanup(archive) }
    private var entries = emptyList<String>()
    private var sessions = emptyList<TripSessionRef>()
    private var selected: String? = null
    private var saving = false
    private var busy = false
    private lateinit var status: TextView
    private lateinit var list: ListView
    private lateinit var export: Button
    private lateinit var clear: Button

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        selected = savedInstanceState?.getString("selected")
        list = ListView(this).apply {
            setOnItemClickListener { _, _, position, _ ->
                if (busy) return@setOnItemClickListener
                startActivity(Intent(context, TripHistoryActivity::class.java)
                    .putExtra(TripHistoryActivity.SESSION_ID, sessions[position].id))
            }
        }
        status = TextView(this).apply {
            text = getString(R.string.trip_journal_loading)
            textSize = 15f
            setPadding(0, 8.dp, 0, 4.dp)
        }
        export = Button(this).apply {
            setText(R.string.trip_journal_export)
            setOnClickListener { chooseExportPart() }
        }
        clear = Button(this).apply {
            setText(R.string.trip_journal_clear)
            setOnClickListener { previewCleanup() }
        }
        setContentView(LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(24.dp, 14.dp, 24.dp, 14.dp)
            addView(TextView(context).apply {
                text = getString(R.string.trip_journal_title)
                textSize = 24f
            })
            addView(status)
            addView(list, LinearLayout.LayoutParams(-1, 0, 1f))
            addView(LinearLayout(context).apply {
                addView(Button(context).apply {
                    setText(R.string.trip_journal_return)
                    setOnClickListener { finish() }
                })
                addView(export)
                addView(clear)
            })
            applySystemBarPadding()
        })
        reloadCatalog()
    }

    private fun reloadCatalog() {
        runArchiveTask(R.string.trip_journal_loading, { archive.entries() to TripHistory(archive).catalog() }) { result ->
            result.fold(
                onSuccess = { (files, trips) ->
                    entries = files
                    sessions = trips
                    val parts = sessions.sumOf { it.parts.size }
                    status.setText(when {
                        entries.isEmpty() -> R.string.trip_journal_empty
                        parts < entries.size -> R.string.trip_journal_partial
                        else -> R.string.trip_journal_choose
                    })
                },
                onFailure = { e ->
                    Log.w("LanosJournal", "Could not list trip parts", e)
                    status.setText(R.string.trip_journal_list_error)
                    entries = emptyList()
                    sessions = emptyList()
                },
            )
            val format = DateTimeFormatter.ofPattern("dd.MM.yyyy HH:mm:ss", Locale.getDefault())
                .withZone(ZoneId.systemDefault())
            val rows = sessions.map { getString(R.string.trip_journal_row, format.format(it.startedAt), it.parts.size) }
            list.adapter = ArrayAdapter(this, android.R.layout.simple_list_item_1, rows)
            updateActions()
        }
    }

    private fun chooseExportPart() {
        val files = entries
        AlertDialog.Builder(this)
            .setTitle(R.string.trip_journal_export_parts)
            .setItems(files.toTypedArray()) { _, position -> chooseDestination(files[position]) }
            .setNegativeButton(android.R.string.cancel, null)
            .show()
    }

    private fun previewCleanup() {
        runArchiveTask(R.string.trip_journal_clear_checking, cleanup::emptySessions) { result ->
            result.onFailure(::showCleanupError).onSuccess { candidates ->
                if (candidates.isEmpty()) {
                    showMessage(getString(R.string.trip_journal_clear_none))
                    return@onSuccess
                }
                AlertDialog.Builder(this)
                    .setTitle(R.string.trip_journal_clear)
                    .setMessage(getString(R.string.trip_journal_clear_confirm, candidates.size))
                    .setNegativeButton(android.R.string.cancel, null)
                    .setPositiveButton(R.string.trip_journal_clear_action) { _, _ -> removeEmpty(candidates) }
                    .show()
            }
        }
    }

    private fun removeEmpty(candidates: List<TripSessionRef>) {
        runArchiveTask(R.string.trip_journal_clear_removing, { cleanup.remove(candidates) }) { result ->
            result.onFailure(::showCleanupError).onSuccess { outcome ->
                val message = if (outcome.skipped == 0) getString(R.string.trip_journal_clear_done, outcome.removed)
                    else getString(R.string.trip_journal_clear_partial, outcome.removed, outcome.skipped)
                showMessage(message)
            }
            reloadCatalog()
        }
    }

    private fun showCleanupError(e: Throwable) {
        Log.w("LanosJournal", "Could not clean up trip journal", e)
        showMessage(getString(R.string.trip_journal_clear_error))
    }

    private fun <T> runArchiveTask(message: Int, work: () -> T, complete: (Result<T>) -> Unit) {
        val prior = status.text
        busy = true
        status.setText(message)
        updateActions()
        TripJournal.io.execute {
            val result = runCatching(work)
            runOnUiThread {
                if (isDestroyed || isFinishing) return@runOnUiThread
                busy = false
                status.text = prior
                updateActions()
                complete(result)
            }
        }
    }

    private fun updateActions() {
        val available = !busy && selected == null && !saving
        list.isEnabled = available
        export.isEnabled = available && entries.isNotEmpty()
        clear.isEnabled = available && sessions.isNotEmpty()
    }

    override fun onSaveInstanceState(outState: Bundle) {
        outState.putString("selected", selected)
        super.onSaveInstanceState(outState)
    }

    @Suppress("DEPRECATION") // Platform activity results keep the native picker dependency-free.
    private fun chooseDestination(name: String) {
        if (busy || selected != null || saving) return
        selected = name
        updateActions()
        try {
            val intent = Intent(Intent.ACTION_CREATE_DOCUMENT)
                .addCategory(Intent.CATEGORY_OPENABLE)
                .setType("application/x-ndjson")
                .putExtra(Intent.EXTRA_TITLE, name)
            startActivityForResult(intent, SAVE_TRIP)
        } catch (_: ActivityNotFoundException) {
            selected = null
            updateActions()
            showMessage(getString(R.string.trip_journal_picker_unavailable))
        }
    }

    @Deprecated("Receives the platform document picker result")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != SAVE_TRIP) return
        val name = selected ?: return
        selected = null
        updateActions()
        if (resultCode != RESULT_OK) return
        val uri = data?.data?.takeIf { it.scheme == "content" } ?: return
        saving = true
        updateActions()
        val resolver = applicationContext.contentResolver
        exportWorker.execute {
            val result = runCatching {
                val output = resolver.openOutputStream(uri, "wt") ?: throw IOException("Destination unavailable")
                output.use { archive.export(name, it) }
            }
            runOnUiThread {
                saving = false
                result.exceptionOrNull()?.let { Log.w("LanosJournal", "Could not export trip", it) }
                if (!isDestroyed && !isFinishing) {
                    updateActions()
                    showMessage(if (result.isSuccess) getString(R.string.trip_journal_saved) else getString(R.string.trip_journal_export_error))
                }
            }
        }
    }

    private fun showMessage(text: String) = Toast.makeText(this, text, Toast.LENGTH_LONG).show()
    private val Int.dp get() = (this * resources.displayMetrics.density + 0.5f).toInt()

    private companion object {
        const val SAVE_TRIP = 1
        val exportWorker = Executors.newSingleThreadExecutor()
    }
}

internal fun View.applySystemBarPadding() {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return
    val left = paddingLeft
    val top = paddingTop
    val right = paddingRight
    val bottom = paddingBottom
    setOnApplyWindowInsetsListener { view, insets ->
        val safeArea = insets.getInsets(WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout())
        view.setPadding(left + safeArea.left, top + safeArea.top, right + safeArea.right, bottom + safeArea.bottom)
        insets
    }
}
