package download.simplevpn.plan

import android.content.Context
import android.util.Log
import org.json.JSONObject

/**
 * Keeps the plan the client is allowed to use, and the last one that actually
 * worked.
 *
 * Two plans rather than one, and that is the whole of this stage. A mistake in
 * settings must not break the product for everybody at once: the newest plan is
 * a candidate until it has been shown to carry traffic, and a candidate that
 * cannot is abandoned in favour of the last one that could. Nobody presses
 * anything.
 *
 * Three rules guard what may be stored at all, and each exists against a
 * specific way an adversary or an accident gets in.
 *
 * A plan whose sequence number is not higher than the last applied is
 * discarded. Somebody who recorded a valid plan cannot serve it again to push a
 * client back onto a node that has since been withdrawn.
 *
 * A plan past its expiry is discarded whatever its number, because a complete
 * reinstall resets the recorded number and the sequence rule alone would then
 * protect nothing.
 *
 * A refresh that fails deletes nothing. A Control Plane that is briefly
 * unreachable must not disconnect people who are already working.
 */
class PlanStore(context: Context) {

    private val prefs = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    sealed interface Outcome {
        data object Accepted : Outcome
        data class Refused(val reason: String) : Outcome
    }

    /** The last sequence number applied, or zero when nothing has been. */
    val lastSeq: Long get() = prefs.getLong(KEY_SEQ, 0L)

    /** How many times the candidate has been tried and found not to work. */
    val candidateFailures: Int get() = prefs.getInt(KEY_FAILURES, 0)

    /**
     * Validates a freshly received plan and stores it as the candidate.
     *
     * A plan that says something different resets the failure count: it is a
     * different plan and deserves its own chance, and carrying the previous
     * one's failures forward would let one bad plan poison every plan after it.
     *
     * A plan that says the same thing does not, however new its number. The
     * server issues a new number on every request, so two plans a second apart
     * carry different numbers and identical instructions. Resetting on the
     * number made the rollback inert: each retry fetched a fresh number, the
     * count went back to zero, and the same broken plan was tried for ever.
     * The count follows what the plan says, not which copy of it this is.
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

        val sameInstructions = plan.fingerprint == prefs.getString(KEY_FINGERPRINT, null)

        return try {
            prefs.edit()
                .putString(KEY_PLAN, payload.toString())
                .putLong(KEY_SEQ, plan.seq)
                .putLong(KEY_STORED_AT, now)
                .putString(KEY_FINGERPRINT, plan.fingerprint)
                .putInt(KEY_FAILURES, if (sameInstructions) candidateFailures else 0)
                .apply()
            Outcome.Accepted
        } catch (t: Throwable) {
            Log.e(TAG, "could not store the plan", t)
            Outcome.Refused("plan could not be stored")
        }
    }

    /** The newest accepted plan, proved or not. */
    fun stored(): ConnectionPlan? = read(KEY_PLAN)

    /** The last plan that was shown to carry traffic. */
    fun knownGood(): ConnectionPlan? = read(KEY_GOOD)

    /**
     * Which plan to connect with.
     *
     * The candidate until it has failed enough times, then the last one known
     * to work. When there is no known good one the candidate is used anyway:
     * an unproven plan is a poor bet, and having none is a certainty.
     */
    fun bestToTry(): Choice {
        val candidate = stored()
        val good = knownGood()

        // The rule itself lives in PlanChoice, so that what is tested is the
        // decision rather than a copy of it. This reads plans; that decides.
        return when (
            PlanChoice.decide(
                candidateSeq = candidate?.seq,
                goodSeq = good?.seq,
                failures = candidateFailures,
                limit = FAILURES_BEFORE_ROLLING_BACK,
            )
        ) {
            PlanChoice.Use.CANDIDATE -> Choice(candidate, Source.CANDIDATE)
            PlanChoice.Use.KNOWN_GOOD -> Choice(good, Source.KNOWN_GOOD)
            PlanChoice.Use.NOTHING -> Choice(null, Source.NOTHING)
        }
    }

    data class Choice(val plan: ConnectionPlan?, val source: Source)

    enum class Source { CANDIDATE, KNOWN_GOOD, NOTHING }

    /**
     * Records that a plan carried traffic.
     *
     * Only a candidate is ever promoted. Promoting whatever happened to work
     * would mean the known good plan drifts to whichever one was tried last,
     * which is not the same thing at all.
     */
    fun proved(source: Source) {
        if (source != Source.CANDIDATE) return
        val candidate = prefs.getString(KEY_PLAN, null) ?: return
        prefs.edit()
            .putString(KEY_GOOD, candidate)
            .putLong(KEY_GOOD_SEQ, lastSeq)
            .putInt(KEY_FAILURES, 0)
            .apply()
    }

    /**
     * Records that a plan did not carry traffic.
     *
     * A candidate that fails is counted, and rolled back from once it has
     * failed enough times.
     *
     * A known good plan that fails is **forgotten**, and that is the more
     * important half. "Known good" means it worked last time, not that it works
     * for ever. Without this, a plan issued to escape a block could be pushed
     * aside by two bad moments on a train, and the device would then cling to
     * the blocked plan it remembered as good - our fix defeated by our own
     * safety net. Forgetting it puts the newest plan back in play.
     */
    fun failed(source: Source) {
        when (source) {
            Source.CANDIDATE ->
                prefs.edit().putInt(KEY_FAILURES, candidateFailures + 1).apply()

            Source.KNOWN_GOOD ->
                prefs.edit().remove(KEY_GOOD).remove(KEY_GOOD_SEQ).putInt(KEY_FAILURES, 0).apply()

            Source.NOTHING -> Unit
        }
    }

    /** Forgets everything usable. Used when the server withdraws a client. */
    fun clear() {
        prefs.edit()
            .remove(KEY_PLAN)
            .remove(KEY_STORED_AT)
            .remove(KEY_GOOD)
            .remove(KEY_GOOD_SEQ)
            .remove(KEY_FAILURES)
            .remove(KEY_FINGERPRINT)
            .apply()
        // The sequence number survives on purpose. Forgetting it would make
        // the client accept a replayed older plan the moment it is offered.
    }

    private fun read(key: String): ConnectionPlan? {
        val raw = prefs.getString(key, null) ?: return null
        return try {
            ConnectionPlan.parse(JSONObject(raw))
        } catch (t: Throwable) {
            Log.w(TAG, "stored plan is unreadable", t)
            null
        }
    }

    private companion object {
        const val TAG = "PlanStore"
        const val NAME = "plan"
        const val KEY_PLAN = "payload"
        const val KEY_SEQ = "seq"
        const val KEY_STORED_AT = "stored_at"
        const val KEY_GOOD = "known_good"
        const val KEY_GOOD_SEQ = "known_good_seq"
        const val KEY_FAILURES = "candidate_failures"
        const val KEY_FINGERPRINT = "candidate_fingerprint"
        const val CLOCK_TOLERANCE_MS = 5 * 60 * 1000L

        /**
         * Two, because one failure is a bad moment on a network and two in a
         * row is a plan. Rolling back on the first would send people back to
         * old settings every time a train went into a tunnel.
         */
        const val FAILURES_BEFORE_ROLLING_BACK = 2
    }
}
