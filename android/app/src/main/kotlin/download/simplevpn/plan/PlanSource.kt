package download.simplevpn.plan

import android.content.Context
import android.util.Log
import download.simplevpn.config.ConnectionProfile

/**
 * Where the client gets its endpoint, now that the build does not carry one.
 *
 * The order below is the whole behaviour of the stage, and each step exists
 * because of a specific way the previous one fails.
 *
 * A stored plan that is still fresh is used without asking anyone. Asking on
 * every connection would make the Control Plane part of the critical path for
 * people who are already provisioned.
 *
 * A stale plan triggers a request. If it succeeds and the answer is signed,
 * newer and unexpired, it replaces what was stored.
 *
 * If the request fails, the stale plan is used anyway. This is the grace period
 * from remote-config.md, and it is what keeps a briefly unreachable Control
 * Plane from disconnecting everyone who is already working. Without it the
 * Control Plane would be a single point of failure for the entire installed
 * base rather than for new devices only.
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
    }

    private val store = PlanStore(context)
    private val client = ControlPlaneClient(context)

    /** Must not be called on the main thread: it may open a connection. */
    fun currentProfile(now: Long = System.currentTimeMillis()): Result {
        val stored = store.stored()

        if (stored != null && now < stored.expiresAt) {
            return Result.Available(stored, stored.primary, "stored plan")
        }

        when (val fetched = fetch(now)) {
            is Result.Available -> return fetched
            is Result.Missing -> Log.i(TAG, "could not refresh: ${fetched.reason}")
        }

        if (stored != null) {
            // Expired, and the server cannot be reached. Using it is the lesser
            // failure: the nodes it names are probably still there, and the
            // alternative is disconnecting somebody over a server outage that
            // has nothing to do with them.
            return Result.Available(stored, stored.primary, "expired plan, control plane unreachable")
        }

        return Result.Missing("no endpoint and the control plane cannot be reached")
    }

    private fun fetch(now: Long): Result {
        val response = when (val answer = client.requestPlan()) {
            is ControlPlaneClient.Result.Received -> answer.envelopeJson
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
