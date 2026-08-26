package download.simplevpn.plan

import android.content.Context
import android.util.Log
import download.simplevpn.auth.AccountStore
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

        /**
         * This installation is no longer recognised.
         *
         * Told apart from every other failure because it means something the
         * person can act on and nothing will fix by itself: somebody signed in
         * on another device, or this one was cut off. Treating it as an outage
         * would leave the application retrying forever against a server that
         * has already given its final answer.
         */
        data object Revoked : Result
    }

    fun requestPlan(): Result {
        // No identifier of any kind is sent. Who this is comes from the secret
        // in the header, which was handed over once when a mailbox was proved.
        // An identifier in the body would be a claim, and a claim is exactly
        // what somebody impersonating this device would supply.
        val token = accounts.deviceToken
            ?: return Result.Failed("this installation is not signed in")

        val body = """
            {"supported_transports":["vless-ws-tls"],
             "app_version":$APP_VERSION}
        """.trimIndent()

        return post(ControlPlane.BASE_URL + "/v1/plan", body, token)
    }

    /**
     * Asks what the server says about the service as a whole.
     *
     * Unauthenticated on purpose. This document says whether anybody may
     * connect at all, and a version of it that only signed-in devices could
     * read would leave the one population that most needs to be told to stop -
     * builds whose sign-in is broken - unable to hear it.
     */
    fun requestConfig(): Result = get(ControlPlane.BASE_URL + "/v1/config")

    /**
     * Asks whether this installation is still one the server knows.
     *
     * Exists because a connected device would otherwise not find out it had
     * been cut off until its plan expired - up to a day of a tunnel that looks
     * established and carries nothing.
     *
     * Deliberately not a plan request. Asking for a plan every few minutes
     * would have the server issue, number and record a document each time, for
     * an answer that is one bit long.
     */
    fun checkStanding(): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        return post(ControlPlane.BASE_URL + "/v1/devices", "{}", token)
    }

    /**
     * Asks what address this device is seen from.
     *
     * The one question a phone cannot answer about itself. Used to notice that
     * the network already runs through one of our nodes, which is what a
     * router running this VPN looks like from the inside: nothing at all.
     */
    fun whereFrom(): String? {
        val token = accounts.deviceToken ?: return null
        val answer = post(ControlPlane.BASE_URL + "/v1/whereami", "{}", token)
        if (answer !is Result.Received) return null
        return try {
            org.json.JSONObject(answer.envelopeJson).optString("address").ifBlank { null }
        } catch (t: Throwable) {
            Log.w(TAG, "cannot read the address", t)
            null
        }
    }

    /**
     * Tells the server that a plan did not work.
     *
     * The whole point of the stage is that our mistake must not break the
     * product for everybody, and rolling back on the device is only half of
     * that: somebody has to learn a plan is failing in the field, or the next
     * person to install gets the same one.
     *
     * Carries a number and a reason and nothing else. What went wrong is
     * useful; who it went wrong for is not.
     */
    fun reportPlanFailure(seq: Long, reason: String) {
        val token = accounts.deviceToken ?: return
        val body = org.json.JSONObject()
            .put("seq", seq)
            .put("reason", reason)
            .toString()
        post(ControlPlane.BASE_URL + "/v1/plan/failed", body, token)
    }

    private fun get(url: String): Result {
        var connection: HttpURLConnection? = null
        return try {
            connection = (URL(url).openConnection() as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = TIMEOUT_MS
                readTimeout = TIMEOUT_MS
                setRequestProperty("accept", "application/json")
            }

            val code = connection.responseCode
            if (code != HttpURLConnection.HTTP_OK) {
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

    private fun post(url: String, body: String, token: String): Result {
        var connection: HttpURLConnection? = null
        return try {
            connection = (URL(url).openConnection() as HttpURLConnection).apply {
                requestMethod = "POST"
                connectTimeout = TIMEOUT_MS
                readTimeout = TIMEOUT_MS
                doOutput = true
                setRequestProperty("content-type", "application/json")
                setRequestProperty("authorization", "Bearer $token")
                // Nothing identifying beyond that. A header naming the
                // application would mark every request as ours to anyone
                // watching the connection metadata.
                setRequestProperty("accept", "application/json")
            }

            connection.outputStream.use { it.write(body.toByteArray()) }

            val code = connection.responseCode

            // The one answer that is final. The secret this device holds is no
            // longer one the server knows, which happens when somebody signs in
            // elsewhere or this device is cut off. Retrying cannot help, and
            // treating it as an outage would leave the person staring at a
            // reconnection that never comes.
            if (code == HttpURLConnection.HTTP_UNAUTHORIZED) return Result.Revoked

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

    private val accounts by lazy { AccountStore(context) }

    private companion object {
        const val TAG = "ControlPlaneClient"
        const val TIMEOUT_MS = 15_000
        const val APP_VERSION = 1
    }
}
