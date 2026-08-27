package download.simplevpn.auth

import android.content.Context
import android.util.Log
import download.simplevpn.plan.Entry
import download.simplevpn.plan.EntryBook
import download.simplevpn.plan.EntryTransport
import org.json.JSONObject
import java.net.HttpURLConnection


/**
 * Signing in, which here means proving you can read a mailbox.
 *
 * There is no password to invent, no password to repeat, no code to copy across
 * from a message and no name to choose. What a password protects is access to
 * the mailbox anyway - every reset flow admits as much - so the mailbox is used
 * directly instead of as a fallback.
 */
class AuthClient(private val context: Context) {

    private val deviceId = DeviceIdentity.of(context).deviceId

    sealed interface StartResult {
        /**
         * Returned whether or not the address has an account, and whether or
         * not a message was actually sent. The application must not become a
         * way of asking whether somebody is a customer.
         */
        data class Sent(val attemptId: String, val resendAfterS: Int, val expiresInS: Int) : StartResult

        data class Malformed(val reason: String) : StartResult
        data class Unreachable(val reason: String) : StartResult
    }

    sealed interface PollResult {
        data class Confirmed(val accountId: String, val deviceToken: String) : PollResult
        data object Pending : PollResult
        data object Expired : PollResult
        data class Unreachable(val reason: String) : PollResult
    }

    fun start(email: String): StartResult {
        val body = JSONObject()
            .put("device_id", deviceId)
            .put("email", email)
            .toString()

        return when (val answer = post("/v1/auth/start", body)) {
            is Answer.Ok -> try {
                val json = JSONObject(answer.body)
                StartResult.Sent(
                    attemptId = json.getString("attempt_id"),
                    resendAfterS = json.optInt("resend_after_s", 60),
                    expiresInS = json.optInt("expires_in_s", 900),
                )
            } catch (t: Throwable) {
                StartResult.Unreachable("answer could not be read")
            }

            is Answer.Refused ->
                // Only shape is refused, never membership.
                StartResult.Malformed("address does not look like an address")

            is Answer.Failed -> StartResult.Unreachable(answer.reason)
        }
    }

    /**
     * Asks whether the link has been followed.
     *
     * Asking, rather than being told, is what makes opening the link on a
     * laptop work: the phone and the laptop never learn about each other, and
     * the only thing they share is a row on the server.
     */
    fun poll(attemptId: String): PollResult {
        val body = JSONObject()
            .put("attempt_id", attemptId)
            .put("device_id", deviceId)
            .toString()

        return when (val answer = post("/v1/auth/poll", body)) {
            is Answer.Ok -> try {
                val json = JSONObject(answer.body)
                when (json.optString("status")) {
                    "confirmed" -> PollResult.Confirmed(
                        accountId = json.getString("account_id"),
                        // The secret this installation authenticates with from
                        // now on. Handed over once, here, because this is the
                        // only exchange the server can be sure reaches the
                        // device that started the sign-in.
                        deviceToken = json.getString("device_token"),
                    )
                    "expired" -> PollResult.Expired
                    else -> PollResult.Pending
                }
            } catch (t: Throwable) {
                PollResult.Unreachable("answer could not be read")
            }

            is Answer.Refused -> PollResult.Expired
            is Answer.Failed -> PollResult.Unreachable(answer.reason)
        }
    }

    private sealed interface Answer {
        data class Ok(val body: String) : Answer
        data object Refused : Answer
        data class Failed(val reason: String) : Answer
    }

    /**
     * Walks the ways in until one answers.
     *
     * Signing in is the one thing a blocked domain must not prevent. A person
     * who cannot sign in has no plan, and without a plan the tunnel that would
     * work around the block cannot be raised either - so a single address here
     * would make a blocked domain permanent for anybody installing the
     * application for the first time.
     */
    private fun post(path: String, body: String): Answer {
        val entries = Entry.order(book.entries()) { bound -> random.nextInt(bound) }
        var last: Answer = Answer.Failed("cannot reach the server")

        for (entry in entries) {
            when (val answer = attempt(entry, path, body)) {
                is Answer.Ok -> return answer

                // The server judged the request rather than failing to receive
                // it. Another way in would produce the same judgement.
                is Answer.Refused -> return answer

                is Answer.Failed -> {
                    last = answer
                    Log.i(TAG, "entry ${entry.kind} did not answer: ${answer.reason}")
                }
            }
        }
        return last
    }

    private fun attempt(entry: Entry, path: String, body: String): Answer {
        var connection: HttpURLConnection? = null
        return try {
            connection = EntryTransport.open(entry, path, TIMEOUT_MS).apply {
                requestMethod = "POST"
                doOutput = true
                setRequestProperty("content-type", "application/json")
                setRequestProperty("accept", "application/json")
            }
            connection.outputStream.use { it.write(body.toByteArray()) }

            when (connection.responseCode) {
                HttpURLConnection.HTTP_OK ->
                    Answer.Ok(connection.inputStream.bufferedReader().readText())

                HttpURLConnection.HTTP_BAD_REQUEST, HttpURLConnection.HTTP_GONE ->
                    Answer.Refused

                else -> Answer.Failed("server refused the request")
            }
        } catch (t: Throwable) {
            // The address is never in this message: it goes to a log.
            Log.w(TAG, "sign-in request failed", t)
            Answer.Failed(t.message ?: "cannot reach the server")
        } finally {
            connection?.disconnect()
        }
    }

    private val book by lazy { EntryBook(context) }
    private val random = java.security.SecureRandom()

    private companion object {
        const val TAG = "AuthClient"

        // Short, because a blocked way in must cost seconds rather than the
        // whole budget for finding one that works.
        const val TIMEOUT_MS = 8_000
    }
}
