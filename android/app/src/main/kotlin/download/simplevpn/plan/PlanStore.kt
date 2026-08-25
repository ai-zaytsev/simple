package download.simplevpn.plan

import android.content.Context
import android.util.Log
import org.json.JSONObject

/**
 * Keeps the plan the client is allowed to use, and refuses the ones it is not.
 *
 * Two rules live here, and both exist to stop an adversary replaying something
 * that was once genuine.
 *
 * A plan whose sequence number is not higher than the last one applied is
 * discarded. Someone who recorded a valid plan cannot serve it again later to
 * push a client back onto a node that has since been withdrawn.
 *
 * A plan past its expiry is discarded whatever its number, because a complete
 * reinstall resets the recorded number and the sequence rule alone would then
 * protect nothing.
 *
 * What this deliberately does not do is delete the last good plan when a
 * refresh fails. A Control Plane that is briefly unreachable must not
 * disconnect people who are already working: the stored plan stays usable, and
 * the decision about how long lives with the caller.
 */
class PlanStore(context: Context) {

    private val prefs = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    sealed interface Outcome {
        data object Accepted : Outcome
        data class Refused(val reason: String) : Outcome
    }

    /** The last sequence number applied, or zero when nothing has been. */
    val lastSeq: Long get() = prefs.getLong(KEY_SEQ, 0L)

    /**
     * Validates a freshly received plan and stores it when it wins.
     *
     * @param now milliseconds since the epoch, passed in so the rule can be
     *   reasoned about rather than depending on when this happens to run
     */
    fun accept(payload: JSONObject, now: Long): Outcome {
        val plan = ConnectionPlan.parse(payload)
            ?: return Outcome.Refused("plan is not usable by this build")

        if (plan.seq <= lastSeq) {
            return Outcome.Refused("plan is older than the one in use")
        }

        // A device clock can be wrong in either direction, so a small tolerance
        // is allowed rather than trusting it exactly. Freshness is really
        // confirmed by the next successful exchange with the server.
        if (plan.expiresAt + CLOCK_TOLERANCE_MS < now) {
            return Outcome.Refused("plan has expired")
        }

        return try {
            prefs.edit()
                .putString(KEY_PLAN, payload.toString())
                .putLong(KEY_SEQ, plan.seq)
                .putLong(KEY_STORED_AT, now)
                .apply()
            Outcome.Accepted
        } catch (t: Throwable) {
            Log.e(TAG, "could not store the plan", t)
            Outcome.Refused("plan could not be stored")
        }
    }

    /** The stored plan, whether or not it has expired. */
    fun stored(): ConnectionPlan? {
        val raw = prefs.getString(KEY_PLAN, null) ?: return null
        return try {
            ConnectionPlan.parse(JSONObject(raw))
        } catch (t: Throwable) {
            Log.w(TAG, "stored plan is unreadable", t)
            null
        }
    }

    /** Forgets everything. Used when the server withdraws a client. */
    fun clear() {
        prefs.edit().remove(KEY_PLAN).remove(KEY_STORED_AT).apply()
        // The sequence number survives on purpose. Forgetting it would make
        // the client accept a replayed older plan the moment it is offered.
    }

    private companion object {
        const val TAG = "PlanStore"
        const val NAME = "plan"
        const val KEY_PLAN = "payload"
        const val KEY_SEQ = "seq"
        const val KEY_STORED_AT = "stored_at"
        const val CLOCK_TOLERANCE_MS = 5 * 60 * 1000L
    }
}
