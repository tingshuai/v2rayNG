package com.v2ray.ang.handler

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkRequest
import android.net.NetworkCapabilities
import com.google.gson.Gson
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.coroutines.withContext
import kotlin.coroutines.resume

/** Sends coarse reports over a validated non-VPN network; requests contain no device or address identity. */
object FreeNodeFeedbackClient {
    private const val ENDPOINT = "https://freenode.go2base.com/feedback"
    private const val MAX_REPORTS = 8
    private const val MAX_BODY_BYTES = 2_048

    suspend fun submit(context: Context, reports: List<FreeNodeFeedbackReport>): Boolean = withContext(Dispatchers.IO) {
        if (!FreeNodeManager.isEnabled() || !FreeNodeManager.isFeedbackEnabled() || reports.isEmpty() || reports.size > MAX_REPORTS) return@withContext false
        val body = Gson().toJson(reports).toByteArray(Charsets.UTF_8)
        if (body.size > MAX_BODY_BYTES) return@withContext false
        val connectivity = context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager ?: return@withContext false
        val network = withTimeoutOrNull(3_000) { validatedNonVpnNetwork(connectivity) } ?: return@withContext false
        run {
            var connection: HttpURLConnection? = null
            try {
                connection = network.openConnection(URL(ENDPOINT)) as? HttpURLConnection ?: return@run
                connection.requestMethod = "POST"
                connection.connectTimeout = 3_000
                connection.readTimeout = 3_000
                connection.doOutput = true
                connection.setRequestProperty("Content-Type", "application/json")
                connection.setRequestProperty("Cache-Control", "no-store")
                connection.setRequestProperty("User-Agent", "FreeV2ray-FN2.0")
                connection.setRequestProperty("Connection", "close")
                connection.outputStream.use { it.write(body) }
                if (connection.responseCode == HttpURLConnection.HTTP_NO_CONTENT) return@withContext true
            } catch (_: Exception) {
                // Feedback is best effort and never changes probe or connection results.
            } finally {
                connection?.disconnect()
            }
        }
        false
    }

    private suspend fun validatedNonVpnNetwork(connectivity: ConnectivityManager): Network? =
        suspendCancellableCoroutine { continuation ->
            val closed = AtomicBoolean(false)
            val callback = object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(network: Network) {
                    acceptIfValidated(network)
                }

                override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) {
                    if (capabilities.isValidatedInternet()) accept(network)
                }

                private fun acceptIfValidated(network: Network) {
                    if (connectivity.getNetworkCapabilities(network)?.isValidatedInternet() == true) accept(network)
                }

                private fun accept(network: Network) {
                    close()
                    if (continuation.isActive) continuation.resume(network)
                }

                override fun onUnavailable() {
                    close()
                    if (continuation.isActive) continuation.resume(null)
                }

                private fun close() {
                    if (closed.compareAndSet(false, true)) runCatching { connectivity.unregisterNetworkCallback(this) }
                }
            }
            try {
                connectivity.registerNetworkCallback(
                    NetworkRequest.Builder()
                        .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                        .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
                        .build(),
                    callback,
                )
                continuation.invokeOnCancellation {
                    if (closed.compareAndSet(false, true)) runCatching { connectivity.unregisterNetworkCallback(callback) }
                }
            } catch (_: Exception) {
                if (continuation.isActive) continuation.resume(null)
            }
        }

    private fun NetworkCapabilities.isValidatedInternet(): Boolean =
        hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) &&
            hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) &&
            !hasTransport(NetworkCapabilities.TRANSPORT_VPN)
}
