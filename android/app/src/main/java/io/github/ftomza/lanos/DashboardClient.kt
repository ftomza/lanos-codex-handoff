package io.github.ftomza.lanos

import android.content.res.AssetManager
import android.graphics.Bitmap
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import java.io.IOException

internal class DashboardClient(
    private val assets: AssetManager,
    private val onLoading: () -> Unit = {},
    private val onLoaded: () -> Unit = {},
    private val onAction: (String) -> Unit = {},
    private val historyReport: ByteArray? = null,
) : WebViewClient() {
    private val document = if (historyReport == null) AssetPolicy.DASHBOARD else AssetPolicy.HISTORY
    private val headers = mapOf(
        "Content-Security-Policy" to "default-src 'none'; script-src 'self'; style-src 'self'; " +
            "img-src data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'" +
            if (historyReport != null) "; connect-src 'self'" else "",
        "X-Content-Type-Options" to "nosniff",
        "Cache-Control" to "no-store",
    )

    override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
        val url = request.url.toString()
        if (historyReport == null) {
            AssetPolicy.action(url, request.method, request.isForMainFrame, request.hasGesture())?.let(onAction)
        }
        return !request.isForMainFrame || request.method != "GET" || url != document
    }

    override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse {
        val url = request.url.toString()
        val name = if (historyReport == null) AssetPolicy.asset(url, request.method)
            else AssetPolicy.historyAsset(url, request.method, request.isForMainFrame)
        if (name == null || (request.isForMainFrame && url != document)) return rejected(403, "Forbidden")
        if (url == AssetPolicy.HISTORY_REPORT && historyReport != null) {
            return WebResourceResponse("application/json", "UTF-8", 200, "OK", headers, historyReport.inputStream())
        }
        val mime = when {
            name.endsWith(".mjs") -> "text/javascript"
            name.endsWith(".css") -> "text/css"
            else -> "text/html"
        }
        val stream = try {
            assets.open(name, AssetManager.ACCESS_STREAMING)
        } catch (_: IOException) {
            return rejected(404, "Not Found")
        }
        return WebResourceResponse(mime, "UTF-8", 200, "OK", headers, stream)
    }

    override fun onPageStarted(view: WebView, url: String, favicon: Bitmap?) {
        onLoading()
    }

    override fun onPageFinished(view: WebView, url: String) {
        if (url == document) onLoaded()
    }

    private fun rejected(code: Int, reason: String) =
        WebResourceResponse("text/plain", "UTF-8", code, reason, headers, null)

}
