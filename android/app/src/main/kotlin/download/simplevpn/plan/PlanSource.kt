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
 * If the answer is signed, newer and unexpired, it becomes the candidate.
 *
 * Which plan is then used is not necessarily that one. A candidate is a
 * proposal until it has been shown to carry traffic; one that cannot is
 * abandoned for the last plan that could. That is what keeps a mistake in
 * settings from breaking the product for everybody at once, and it happens
 * without anybody pressing anything.
 *
 * If the request fails, what is stored is used, fresh or stale. This is the
 * grace period from remote-config.md, and it is what keeps a briefly
 * unreachable Control Plane from disconnecting everyone who is already
 * working. Asking first does not put the Control Plane on the critical path,
 * because failing to reach it changes nothing about the outcome.
 *
 * If the server answers that it does not know this device, that is final and
 * nothing stored is reached for. Somebody has signed in elsewhere, or this
 * device was cut off; retrying cannot change it.
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
         * Separate from [Missing] because nothing stored must be used to paper
         * over it. A revoked device still holds a plan that looks perfectly
         * good and a credential no node will accept: connecting on it would
         * produce a tunnel that carries nothing and an explanation nobody can
         * find.
         */
        data object Revoked : Result
    }

    private val store = PlanStore(context)
    private val client = ControlPlaneClient(context)

    /** Must not be called on the main thread: it may open a connection. */
    fun currentProfile(now: Long = System.currentTimeMillis()): Result {
        val refreshed = refresh(now)
        if (refreshed == Refresh.REVOKED) return Result.Revoked

        val choice = store.bestToTry()
        val plan = choice.plan
            ?: return Result.Missing("no endpoint and the control plane cannot be reached")

        val reachedServer = refreshed == Refresh.UPDATED || refreshed == Refresh.UNCHANGED
        val source = when {
            choice.source == PlanStore.Source.KNOWN_GOOD ->
                "last plan known to work, after ${store.candidateFailures} failed attempts on the newest"

            refreshed == Refresh.UPDATED -> "fresh plan"
            reachedServer -> "stored plan"
            else -> "stored plan, control plane unreachable"
        }

        return Result.Available(plan, plan.primary, source)
    }

    /** Which of the two plans the last connection used. */
    fun sourceInUse(): PlanStore.Source = store.bestToTry().source

    /** Records that the plan in use carried traffic. */
    fun proved(source: PlanStore.Source) = store.proved(source)

    /**
     * Records that it did not.
     *
     * Which plan failed decides what happens next, and the difference matters:
     * a candidate that fails is rolled back from, a known good one that fails
     * is forgotten. See PlanStore.failed.
     */
    fun failed(source: PlanStore.Source) = store.failed(source)

    /** The number of the plan currently proposed, for reporting a bad one. */
    fun candidateSeq(): Long = store.lastSeq

    /**
     * What address this device is seen from, or null when nobody could say.
     *
     * The one question a phone cannot answer about itself, and the only way to
     * notice that its network already runs through one of our nodes.
     *
     * Must not be called on the main thread: it opens a connection.
     */
    fun seenFrom(): String? = client.whereFrom()

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

    /** Tells the server a plan did not work, so somebody can look at it. */
    fun reportFailure(seq: Long, reason: String) {
        client.reportPlanFailure(seq, reason)
    }

    private enum class Refresh { UPDATED, UNCHANGED, UNREACHABLE, REVOKED }

    private fun refresh(now: Long): Refresh {
        val response = when (val answer = client.requestPlan()) {
            is ControlPlaneClient.Result.Received -> answer.envelopeJson
            is ControlPlaneClient.Result.Revoked -> return Refresh.REVOKED
            is ControlPlaneClient.Result.Failed -> {
                Log.i(TAG, "could not refresh: ${answer.reason}")
                return Refresh.UNREACHABLE
            }
        }

        // Signature first, always. Everything below this line trusts the
        // contents, and that trust comes from here and nowhere else.
        val payload = when (val opened = SignedDocument.open(response)) {
            is SignedDocument.Result.Trusted -> opened.payload
            is SignedDocument.Result.Rejected -> {
                Log.w(TAG, "document rejected: ${opened.reason}")
                return Refresh.UNREACHABLE
            }
        }

        return when (val outcome = store.accept(payload, now)) {
            is PlanStore.Outcome.Accepted -> Refresh.UPDATED
            is PlanStore.Outcome.Refused -> {
                Log.i(TAG, "plan not taken: ${outcome.reason}")
                Refresh.UNCHANGED
            }
        }
    }

    private companion object {
        const val TAG = "PlanSource"
    }
}
