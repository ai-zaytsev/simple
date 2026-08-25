package download.simplevpn.plan

import android.content.Context
import android.util.Log
import java.net.HttpURLConnection
import java.net.URL
import java.util.UUID

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
        val body = """
            {"device_id":"${identity.deviceId}",
             "account_id":"${identity.accountId}",
             "supported_transports":["vless-ws-tls"],
             "app_version":$APP_VERSION}
        """.trimIndent()

        return post("$BASE_URL/v1/plan", body)
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

        /**
         * The one address compiled into the build. See the class comment for
         * why it is a name and why that is safe here and nowhere else.
         */
        const val BASE_URL = "https://simple-syncbridge.download"
        const val TIMEOUT_MS = 15_000
        const val APP_VERSION = 1
    }
}

/**
 * Who this installation is, as far as the Control Plane is concerned.
 *
 * Generated once and kept. Not derived from anything about the hardware: an
 * identifier tied to the device would follow the person across reinstalls and
 * survive their deleting the application, which is exactly what the privacy
 * model forbids.
 */
internal class DeviceIdentity private constructor(
    val deviceId: String,
    val accountId: String,
) {
    companion object {
        fun of(context: Context): DeviceIdentity {
            val prefs = context.getSharedPreferences("identity", Context.MODE_PRIVATE)

            val device = prefs.getString(KEY_DEVICE, null) ?: UUID.randomUUID().toString()
            // Until accounts exist, a device is its own account. The two are
            // separate fields from the start because merging them would be
            // impossible to undo once installations exist in the wild.
            val account = prefs.getString(KEY_ACCOUNT, null) ?: UUID.randomUUID().toString()

            prefs.edit().putString(KEY_DEVICE, device).putString(KEY_ACCOUNT, account).apply()
            return DeviceIdentity(device, account)
        }

        private const val KEY_DEVICE = "device_id"
        private const val KEY_ACCOUNT = "account_id"
    }
}
