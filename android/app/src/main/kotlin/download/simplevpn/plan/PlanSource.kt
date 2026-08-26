package download.simplevpn.plan

import android.content.Context
import android.util.Log
import download.simplevpn.config.ConnectionProfile

/**
 * Where the client gets its endpoint, now that the build does not carry one.
 *
 * The order below is the whole behaviour, and each step exists because of a
 * specific way the previous one fails.
 *
 * The server is asked on every connection. It used to be asked only when the
 * stored plan had gone stale, which looked like a saving and was a hole: a
 * device cut off while holding a plan that still looked good would connect on
 * a credential no node accepts, report success, and carry nothing.
 *
 * If the answer is signed, newer and unexpired, it replaces what was stored.
 *
 * If the request fails, the stored plan is used, fresh or stale. This is the
 * grace period from remote-config.md, and it is what keeps a briefly
 * unreachable Control Plane from disconnecting everyone who is already
 * working. Asking first does not put the Control Plane on the critical path,
 * because failing to reach it changes nothing about the outcome.
 *
 * If the server answers that it does not know this device, that is final and
 * the stored plan is not reached for. Somebody has signed in elsewhere, or this
 * device was cut off; retrying cannot change it.
 *
 * Only when there is nothing stored and nothing can be fetched does the client
 * say it has no endpoint, which is true and is better than failing obscurely
 * later.
 */
class PlanSource(private val context: Context) {

    sealed interface Result {
        data class Available(
            val plan: ConnectionPlan,
            val profile: ConnectionProfile,
            val source: String,
        ) : Result
        data class Missing(val reason: String) : Result

        /**
         * The server no longer knows this installation.
         *
         * Separate from [Missing] because the stored plan must not be used to
         * paper over it. A revoked device still holds a plan that looks
         * perfectly good and a credential no node will accept: connecting on it
         * would produce a tunnel that carries nothing and an explanation
         * nobody can find.
         */
        data object Revoked : Result
    }

    private val store = PlanStore(context)
    private val client = ControlPlaneClient(context)

    /** Must not be called on the main thread: it may open a connection. */
    fun currentProfile(now: Long = System.currentTimeMillis()): Result {
        val stored = store.stored()

        // Asked every time, and the stored plan is what happens when asking
        // fails - not what happens instead of asking.
        //
        // The difference matters because a device can be cut off while holding
        // a plan that still looks perfectly good. Not asking would let it
        // connect on a credential no node accepts: a tunnel that reports
        // success and carries nothing, with the explanation sitting unread on
        // the server. The grace period below is untouched; the Control Plane
        // is still not on the critical path when it cannot be reached.
        when (val fetched = fetch(now)) {
            is Result.Available -> return fetched

            // Final, so the stored plan is not reached for. See Revoked.
            is Result.Revoked -> return Result.Revoked

            is Result.Missing -> Log.i(TAG, "could not refresh: ${fetched.reason}")
        }

        if (stored != null) {
            // The server cannot be reached. Using what is stored is the lesser
            // failure: the nodes it names are probably still there, and the
            // alternative is disconnecting somebody over a server outage that
            // has nothing to do with them.
            val age = if (now < stored.expiresAt) "stored plan" else "expired plan"
            return Result.Available(stored, stored.primary, "$age, control plane unreachable")
        }

        return Result.Missing("no endpoint and the control plane cannot be reached")
    }

    /** What the server says about this installation still being known. */
    enum class Standing { KNOWN, REVOKED, UNREACHABLE }

    /**
     * Asks whether this installation is still recognised, and nothing else.
     *
     * A connected device would otherwise not learn it had been cut off until
     * its plan expired - up to a day of a tunnel that reports success and
     * carries nothing.
     *
     * Unreachable is not revoked. A server that cannot be reached says nothing
     * about whether this device is welcome, and treating silence as a refusal
     * would let anyone disconnect everybody by blocking one address.
     *
     * Must not be called on the main thread: it opens a connection.
     */
    fun standing(): Standing = when (client.checkStanding()) {
        is ControlPlaneClient.Result.Received -> Standing.KNOWN
        is ControlPlaneClient.Result.Revoked -> Standing.REVOKED
        is ControlPlaneClient.Result.Failed -> Standing.UNREACHABLE
    }

    private fun fetch(now: Long): Result {
        val response = when (val answer = client.requestPlan()) {
            is ControlPlaneClient.Result.Received -> answer.envelopeJson
            is ControlPlaneClient.Result.Revoked -> return Result.Revoked
            is ControlPlaneClient.Result.Failed -> return Result.Missing(answer.reason)
        }

        // Signature first, always. Everything below this line trusts the
        // contents, and that trust comes from here and nowhere else.
        val payload = when (val opened = SignedDocument.open(response)) {
            is SignedDocument.Result.Trusted -> opened.payload
            is SignedDocument.Result.Rejected -> {
                Log.w(TAG, "document rejected: ${opened.reason}")
                return Result.Missing(opened.reason)
            }
        }

        return when (val outcome = store.accept(payload, now)) {
            is PlanStore.Outcome.Accepted ->
                store.stored()?.let { Result.Available(it, it.primary, "fresh plan") }
                    ?: Result.Missing("plan stored but unreadable")

            is PlanStore.Outcome.Refused -> Result.Missing(outcome.reason)
        }
    }

    private companion object {
        const val TAG = "PlanSource"
    }
}
