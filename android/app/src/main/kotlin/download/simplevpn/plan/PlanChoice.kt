package download.simplevpn.plan

/**
 * Which of the two remembered plans to connect with.
 *
 * Lifted out of the store so that the rule is one thing rather than two: the
 * store keeps plans, this decides between them, and a test can exercise the
 * decision itself rather than a copy of it written to agree with it.
 *
 * Everything decided here is invisible from the outside. A client on the wrong
 * plan connects, reports success and carries nothing; a client clinging to old
 * settings sits blocked while the fix waits unused on the server. Neither shows
 * up as a failure on any screen, which is why the rule is stated in one place
 * and checked.
 */
object PlanChoice {

    enum class Use { CANDIDATE, KNOWN_GOOD, NOTHING }

    /**
     * @param candidateSeq the newest accepted plan, or null when there is none
     * @param goodSeq the last plan shown to carry traffic, or null
     * @param failures how many times the candidate has been tried and failed
     * @param limit how many failures are allowed before rolling back
     */
    fun decide(candidateSeq: Long?, goodSeq: Long?, failures: Int, limit: Int): Use {
        if (candidateSeq == null) {
            return if (goodSeq == null) Use.NOTHING else Use.KNOWN_GOOD
        }

        // One failure is a bad moment on a network; two in a row is a plan.
        // Rolling back on the first would send people to old settings every
        // time a train went into a tunnel.
        if (failures < limit) return Use.CANDIDATE

        // Rolling back to the same plan is not a rollback. Once a candidate has
        // been proved, the known good plan is that same plan, and reporting a
        // rollback here would name one that never happened.
        if (goodSeq == null || goodSeq >= candidateSeq) return Use.CANDIDATE

        return Use.KNOWN_GOOD
    }

    /**
     * Whether another attempt would try anything the failed one did not.
     *
     * [decide] says which plan to use; this says whether using it again is
     * worth anything. The two belong together because apart they drifted, and
     * the drift was invisible from either side.
     *
     * The service used to rebuild the tunnel after every failed proof and give
     * up only once [decide] answered KNOWN_GOOD. In the two commonest shapes it
     * never answers that: with nothing proven there is nothing to roll back to,
     * and once a candidate has been proved the proven plan *is* the candidate.
     * Both are stated in the tests for [decide], and neither was read as a
     * statement about when to stop trying. So a phone whose node had stopped
     * answering rebuilt its tunnel for ever, a few seconds per turn, saying
     * "reconnecting" each time.
     *
     * @param failures the candidate's failures, counting the one just recorded
     * @param justFailed which plan the attempt that just failed used
     */
    fun worthRetrying(
        candidateSeq: Long?,
        goodSeq: Long?,
        failures: Int,
        limit: Int,
        justFailed: Use,
    ): Boolean {
        // The last plan known to work has now failed as well. Nothing older is
        // kept, and the candidate spent its attempts on the way here.
        if (justFailed != Use.CANDIDATE) return false

        val next = decide(candidateSeq, goodSeq, failures, limit)
        if (next == Use.NOTHING) return false

        // Something different to try: the rollback, which is the whole reason
        // a failed proof does not simply give up.
        if (next != justFailed) return true

        // The same plan again. Worth one more go while it still has attempts,
        // and nothing at all once it does not - trying it a third time on the
        // same network in the same minute is how the loop began.
        return failures < limit
    }
}
