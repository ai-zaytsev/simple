package download.simplevpn.plan

import android.content.Context
import android.util.Log
import download.simplevpn.auth.AccountStore
import java.net.HttpURLConnection
import java.security.SecureRandom

/**
 * Asks the Control Plane, by whichever way still works.
 *
 * There is no single address any more, and that is the point of this stage: a
 * product whose settings arrive down one path stops working the day that path
 * is blocked, and the only cure is a new build - slower than any block and
 * never reaching everybody.
 *
 * The ways are tried in a weighted random order. Random rather than fixed
 * because one order for every client is a signature, and because a fixed first
 * choice puts the entire installed base on one entry. Each attempt gets a short
 * timeout, so a blocked entry costs seconds rather than a hung application.
 *
 * A refusal is not a failure of the path. When the service answers that it does
 * not know this device, that answer arrived, and trying another way in would
 * only ask a different door the same question.
 */
class ControlPlaneClient(
    private val context: Context,
    /**
     * When set, every request goes out through the engine rather than over the
     * ordinary network.
     *
     * The tunnel channel. A device whose every public way in is blocked still
     * has one thing that works: the plan it last used. Raising the tunnel on it
     * and asking from the inside reaches a service that cannot be reached from
     * outside, and the answer carries the new ways in - which is how a block
     * repairs itself without anybody installing anything.
     */
    private val throughTunnel: java.net.Proxy? = null,
) {

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

        return send("/v1/plan", body, token)
    }

    /**
     * Asks what the server says about the service as a whole.
     *
     * Unauthenticated on purpose. This document says whether anybody may
     * connect at all, and a version of it that only signed-in devices could
     * read would leave the one population that most needs to be told to stop -
     * builds whose sign-in is broken - unable to hear it.
     */
    fun requestConfig(): Result = send("/v1/config", null, null)

    /**
     * Asks for the list of ways back in.
     *
     * The one document that has to survive its own subject being blocked, which
     * is why it is fetched by every path in turn and mirrored outside our
     * infrastructure. Unauthenticated, because a device that cannot sign in is
     * exactly the device that needs it.
     */
    fun requestBootstrap(): Result = send("/v1/bootstrap", null, null)

    /**
     * Asks whether this installation is still one the server knows.
     *
     * Deliberately not a plan request. Asking for a plan every few minutes
     * would have the server issue, number and record a document each time, for
     * an answer that is one bit long.
     */
    fun checkStanding(): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        return send("/v1/devices", "{}", token)
    }

    /** Asks what address this device is seen from. */
    fun whereFrom(): String? {
        val token = accounts.deviceToken ?: return null
        val answer = send("/v1/whereami", "{}", token)
        if (answer !is Result.Received) return null
        return try {
            org.json.JSONObject(answer.envelopeJson).optString("address").ifBlank { null }
        } catch (t: Throwable) {
            Log.w(TAG, "cannot read the address", t)
            null
        }
    }

    /**
     * The account's status, as the service knows it right now.
     *
     * Asked rather than remembered from sign-in: a status changes
     * without this installation doing anything - somebody is given VIP
     * while their phone sits in a pocket - and an application showing
     * yesterday's answer would hide a section its owner has paid for.
     *
     * Null when the service could not be reached. Not FREE: silence is
     * not a downgrade, and treating it as one would take the section
     * away every time the network hiccuped.
     */
    fun tier(): Standing? {
        val token = accounts.deviceToken ?: return null
        val answer = send("/v1/devices", "{}", token)
        if (answer !is Result.Received) return null
        return try {
            val json = org.json.JSONObject(answer.envelopeJson)
            val tier = json.optString("tier").ifBlank { return null }
            val buy = json.optJSONObject("purchase")
            Standing(
                tier = tier,
                mayBuy = buy?.optBoolean("available") ?: false,
                whyNot = buy?.optString("reason").orEmpty(),
                // Passed through as it arrived. The phone does not parse the
                // date and does not compare it with its own clock: a device
                // whose clock is wrong would otherwise decide for itself that
                // the wait is over, which is the one thing the server is here
                // to decide.
                opensOn = buy?.optString("available_at").orEmpty(),
            )
        } catch (t: Throwable) {
            Log.w(TAG, "cannot read the standing", t)
            null
        }
    }

    /**
     * What this account is, and whether it may buy what it is not.
     *
     * Both in one answer because they arrive in one call and are drawn in one
     * corner of one screen. Split apart they would be two requests that can
     * disagree, and the disagreement would show as a button that offers
     * something the account already has.
     */
    data class Standing(
        val tier: String,
        val mayBuy: Boolean,
        val whyNot: String,
        val opensOn: String,
    )

    /** Every external device on this account, with its links. */
    fun externalDevices(): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        return send("/v1/external/links", "{}", token)
    }

    /** Connects a router, a television or a computer under a name. */
    fun addExternalDevice(label: String): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        val body = org.json.JSONObject().put("label", label).toString()
        return send("/v1/external", body, token)
    }

    /** Replaces the link of one device, keeping its name and place. */
    fun replaceExternalLink(deviceId: String): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        val body = org.json.JSONObject().put("device_id", deviceId).toString()
        return send("/v1/external/rotate", body, token)
    }

    /** Cuts one device off and leaves every other one alone. */
    fun revokeDevice(deviceId: String): Result {
        val token = accounts.deviceToken ?: return Result.Revoked
        val body = org.json.JSONObject().put("device_id", deviceId).toString()
        return send("/v1/devices/revoke", body, token)
    }

    /**
     * Sends what this device saw of the service since the last report.
     *
     * Totals, and outcomes of trying our own addresses. Nothing about what
     * travelled through the tunnel: see metrics/ServiceReport.kt for why this
     * device is the one that has to answer the blocking question, and for what
     * it is allowed to say.
     */
    fun sendReport(body: String) {
        val token = accounts.deviceToken ?: return
        send("/v1/app/report", body, token)
    }

    /** Tells the server a plan did not work, so somebody can look at it. */
    fun reportPlanFailure(seq: Long, reason: String) {
        val token = accounts.deviceToken ?: return
        val body = org.json.JSONObject()
            .put("seq", seq)
            .put("reason", reason)
            .toString()
        send("/v1/plan/failed", body, token)
    }

    /**
     * Walks the ways in until one answers.
     *
     * @param body null for a request that carries none, which is also how a
     *   GET is asked for
     */
    private fun send(path: String, body: String?, token: String?): Result {
        val entries = Entry.order(book.entries()) { bound -> random.nextInt(bound) }
        if (entries.isEmpty()) return Result.Failed("no way to reach the service")

        var lastReason = "no way to reach the service"
        for (entry in entries) {
            when (val answer = attempt(entry, path, body, token)) {
                is Result.Received -> return answer

                // An answer, not a failure of the path. Another door would
                // give the same one.
                is Result.Revoked -> return answer

                is Result.Failed -> {
                    lastReason = answer.reason
                    Log.i(TAG, "entry ${entry.kind} ${entry.host} did not answer: ${answer.reason}")
                }
            }
        }
        return Result.Failed(lastReason)
    }

    private fun attempt(entry: Entry, path: String, body: String?, token: String?): Result {
        var connection: HttpURLConnection? = null
        return try {
            connection = EntryTransport.open(entry, path, TIMEOUT_MS, throughTunnel).apply {
                requestMethod = if (body == null) "GET" else "POST"
                // Nothing identifying beyond what the body carries. A header
                // naming the application would mark every request as ours to
                // anyone watching the connection metadata.
                setRequestProperty("accept", "application/json")
                if (body != null) {
                    doOutput = true
                    setRequestProperty("content-type", "application/json")
                }
                if (token != null) setRequestProperty("authorization", "Bearer $token")
            }

            if (body != null) {
                connection.outputStream.use { it.write(body.toByteArray()) }
            }

            val code = connection.responseCode

            // The one answer that is final. The secret this device holds is no
            // longer one the server knows, which happens when somebody signs in
            // elsewhere or this device is cut off. Retrying cannot help.
            if (code == HttpURLConnection.HTTP_UNAUTHORIZED) return Result.Revoked

            if (code != HttpURLConnection.HTTP_OK) {
                // The error body is deliberately not surfaced: it is written by
                // whoever answered, and this runs before the signature check
                // that decides whether that was us.
                return Result.Failed("answered $code")
            }

            Result.Received(connection.inputStream.bufferedReader().readText())
        } catch (t: Throwable) {
            Result.Failed(t.message ?: "unreachable")
        } finally {
            connection?.disconnect()
        }
    }

    private val accounts by lazy { AccountStore(context) }
    private val book by lazy { EntryBook(context) }
    private val random = SecureRandom()

    private companion object {
        const val TAG = "ControlPlaneClient"

        // Short, because a blocked entry must cost seconds rather than the
        // whole budget for finding a way in.
        const val TIMEOUT_MS = 8_000
        const val APP_VERSION = 1
    }
}
