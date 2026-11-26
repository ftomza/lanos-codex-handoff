package io.github.ftomza.lanos

internal object AssetPolicy {
    const val ORIGIN = "https://appassets.androidplatform.net"
    const val DASHBOARD = "$ORIGIN/"
    const val PROBE = "${DASHBOARD}vgate-check"
    const val LIVE = "${DASHBOARD}live"
    const val FUEL_IAT = "${DASHBOARD}fuel-iat"
    const val VGATE_POWER_CYCLE = "${DASHBOARD}vgate-power-cycle"
    const val TRIP_JOURNAL = "${DASHBOARD}trip-journal"
    const val HISTORY = "${DASHBOARD}trip-history.html"
    const val HISTORY_REPORT = "${DASHBOARD}trip-history-report.json"
    const val SIMULATOR = "${DASHBOARD}simulator"

    private val bundled = setOf(
        "style.css", "android.css", "app.mjs", "model.mjs", "feed.mjs",
        "fuel.mjs", "fuel-controls.mjs", "mil.mjs", "dtc.mjs", "dtc.css",
    )

    fun asset(url: String, method: String): String? {
        if (method != "GET" || !url.startsWith(DASHBOARD)) return null
        val path = url.removePrefix(DASHBOARD)
        if (path.isEmpty()) return "index.html"
        return path.takeIf(bundled::contains)
    }

    fun historyAsset(url: String, method: String, mainFrame: Boolean): String? {
        if (method != "GET" || !url.startsWith(DASHBOARD)) return null
        if (mainFrame) return "trip-history.html".takeIf { url == HISTORY }
        return url.removePrefix(DASHBOARD).takeIf(historyAssets::contains)
    }

    private val historyAssets = setOf(
        "style.css", "trip-history.css", "trip-history.mjs", "history-chart.mjs",
        "analytics-model.mjs", "trip-history-report.json", "dtc.mjs", "dtc.css",
    )

    fun action(url: String, method: String, mainFrame: Boolean, gesture: Boolean): String? =
        url.takeIf { it in actions && method == "GET" && mainFrame && gesture }

    private val actions = setOf(PROBE, LIVE, SIMULATOR, FUEL_IAT, TRIP_JOURNAL, VGATE_POWER_CYCLE)
}
