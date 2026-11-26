package io.github.ftomza.lanos

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AssetPolicyTest {
    @Test fun dashboardActionsRequireAnExplicitLocalMainFrameTap() {
        for (url in listOf(AssetPolicy.PROBE, AssetPolicy.LIVE, AssetPolicy.SIMULATOR, AssetPolicy.FUEL_IAT, AssetPolicy.TRIP_JOURNAL, AssetPolicy.VGATE_POWER_CYCLE)) {
            assertEquals(url, AssetPolicy.action(url, "GET", mainFrame = true, gesture = true))
            assertNull(AssetPolicy.action(url, "GET", mainFrame = true, gesture = false))
            assertNull(AssetPolicy.action(url, "GET", mainFrame = false, gesture = true))
            assertNull(AssetPolicy.action(url, "POST", mainFrame = true, gesture = true))
            assertNull(AssetPolicy.action("$url?run", "GET", mainFrame = true, gesture = true))
            assertNull(AssetPolicy.asset(url, "GET"))
        }
    }
    @Test fun servesOnlyBundledFilesAtTheirExactUrls() {
        assertEquals("index.html", AssetPolicy.asset("${AssetPolicy.ORIGIN}/", "GET"))
        assertEquals("app.mjs", AssetPolicy.asset("${AssetPolicy.ORIGIN}/app.mjs", "GET"))
        assertEquals("android.css", AssetPolicy.asset("${AssetPolicy.ORIGIN}/android.css", "GET"))
    }

    @Test fun archivedReportsAreOnlyServedAsLocalHistorySubresources() {
        val url = AssetPolicy.HISTORY_REPORT
        assertEquals("trip-history.html", AssetPolicy.historyAsset(AssetPolicy.HISTORY, "GET", mainFrame = true))
        assertEquals("history-chart.mjs", AssetPolicy.historyAsset(AssetPolicy.DASHBOARD + "history-chart.mjs", "GET", mainFrame = false))
        assertEquals("trip-history-report.json", AssetPolicy.historyAsset(url, "GET", mainFrame = false))
        for (bad in listOf(url + "?session=other", url.replace("https://", "http://"), "https://example.org/trip-history-report.json")) {
            assertNull(AssetPolicy.historyAsset(bad, "GET", mainFrame = false))
        }
        assertNull(AssetPolicy.historyAsset(url, "GET", mainFrame = true))
        assertNull(AssetPolicy.historyAsset(url, "POST", mainFrame = false))
        assertNull(AssetPolicy.asset(url, "GET"))
        assertNull(AssetPolicy.historyAsset(AssetPolicy.LIVE, "GET", mainFrame = true))
    }

    @Test fun rejectsNavigationAndRequestsOutsideTheAssetList() {
        for (url in listOf(
            "https://example.org/app.mjs",
            "${AssetPolicy.ORIGIN}.example.org/app.mjs",
            "${AssetPolicy.ORIGIN}:443/app.mjs",
            "${AssetPolicy.ORIGIN}/../app.mjs",
            "${AssetPolicy.ORIGIN}/%61pp.mjs",
            "${AssetPolicy.ORIGIN}/app.mjs?other",
            "${AssetPolicy.ORIGIN}/analytics.html",
            "${AssetPolicy.ORIGIN}/events",
            "file:///android_asset/app.mjs",
            "content://app/app.mjs",
        )) {
            assertNull(url, AssetPolicy.asset(url, "GET"))
        }
        assertNull(AssetPolicy.asset("${AssetPolicy.ORIGIN}/app.mjs", "POST"))
    }
}
