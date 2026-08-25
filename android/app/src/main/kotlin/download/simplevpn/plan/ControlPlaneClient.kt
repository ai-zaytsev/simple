package download.simplevpn.plan

import android.content.Context
import android.util.Log
import download.simplevpn.auth.DeviceIdentity
import java.net.HttpURLConnection
import java.net.URL

/**
 * Asks the Control Plane where to connect.
 *
 * The address of this service is the one thing that must be in the build,
 * because it is how the build finds everything else. It is a name rather than
 * an address on purpose: moving the Control Plane to another machine is then a
 * DNS change instead of an update to every installation.
 *
 * A name works here and would not work for a node. This request happens before
 * any tunnel exists, over the phone's ordinary network, where the system
 * resolver behaves normally. A node is dialled by the engine while the tunnel
 * is being established, at which point the resolver points inside a tunnel that
 * is not up yet - which is why ADR-028 requires node addresses to be numeric.
 */
class ControlPlaneClient(private val context: Context) {

    sealed interface Result {
        data class Received(val envelopeJson: String) : Result
        data class Failed(val reason: String) : Result
    }

    fun requestPlan(): Result {
        // No account identifier is sent. The server takes it from what this
        // device has already proved by following a link in a mailbox; a value
        // in the request would be a claim, and a claim anybody could make.
        val body = """
            {"device_id":"${identity.deviceId}",
             "supported_transports":["vless-ws-tls"],
             "app_version":$APP_VERSION}
        """.trimIndent()

        return post(ControlPlane.BASE_URL + "/v1/plan", body)
    }

    private fun post(url: String, body: String): Result {
        var connection: HttpURLConnection? = null
        return try {
            connection = (URL(url).openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                connectTimeout = TIMEOUT_MS
                readTimeout = TIMEOUT_MS
                doOutput = true
                setRequestProperty("content-type", "application/json")
                // Nothing identifying beyond what the body already carries. A
                // header naming the application would mark every request as
                // ours to anyone watching the connection metadata.
                setRequestProperty("accept", "application/json")
            }

            connection.outputStream.use { it.write(body.toByteArray()) }

            val code = connection.responseCode
            if (code != HttpURLConnection.HTTP_OK) {
                // The error body is deliberately not surfaced to the user: it
                // is written by whoever answered, and this runs before the
                // signature check that decides whether that was us.
                Log.w(TAG, "control plane answered $code")
                return Result.Failed("server refused the request")
            }

            Result.Received(connection.inputStream.bufferedReader().readText())
        } catch (t: Throwable) {
            Log.w(TAG, "control plane unreachable", t)
            Result.Failed(t.message ?: "control plane unreachable")
        } finally {
            connection?.disconnect()
        }
    }

    private val identity by lazy { DeviceIdentity.of(context) }

    private companion object {
        const val TAG = "ControlPlaneClient"
        const val TIMEOUT_MS = 15_000
        const val APP_VERSION = 1
    }
}
