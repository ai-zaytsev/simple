package download.simplevpn.plan

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Which plan is used, and what happens when one stops working.
 *
 * These exercise the rule the client actually applies, not a copy of it. That
 * distinction has cost this project a day already: a check written to agree
 * with the code it checks passes for ever and catches nothing.
 *
 * Everything decided here is invisible from the outside. A client on the wrong
 * plan connects, reports success and carries nothing; a client clinging to old
 * settings sits blocked while the fix waits unused on the server. Neither
 * appears as a failure on any screen.
 */
class PlanChoiceTest {

    private fun decide(candidate: Long?, good: Long?, failures: Int) =
        PlanChoice.decide(candidateSeq = candidate, goodSeq = good, failures = failures, limit = 2)

    @Test
    fun `a fresh candidate is used`() {
        assertEquals(PlanChoice.Use.CANDIDATE, decide(candidate = 5, good = 4, failures = 0))
    }

    @Test
    fun `one failure is not enough to roll back`() {
        assertEquals(PlanChoice.Use.CANDIDATE, decide(candidate = 5, good = 4, failures = 1))
    }

    @Test
    fun `enough failures roll back to the last plan that worked`() {
        assertEquals(PlanChoice.Use.KNOWN_GOOD, decide(candidate = 5, good = 4, failures = 2))
    }

    @Test
    fun `with nothing to roll back to the candidate is used anyway`() {
        // An unproven plan is a poor bet; having none is a certainty.
        assertEquals(PlanChoice.Use.CANDIDATE, decide(candidate = 5, good = null, failures = 9))
    }

    @Test
    fun `rolling back to the same plan is not a rollback`() {
        assertEquals(PlanChoice.Use.CANDIDATE, decide(candidate = 5, good = 5, failures = 9))
    }

    @Test
    fun `a plan newer than the known good one is still preferred after rollback`() {
        // The known good plan is older by definition once a newer one has been
        // accepted, so this is the ordinary rollback case stated the other way
        // round: the newer number never loses to an older one except through
        // failures.
        assertEquals(PlanChoice.Use.CANDIDATE, decide(candidate = 9, good = 4, failures = 0))
    }

    @Test
    fun `nothing stored means nothing to use`() {
        assertEquals(PlanChoice.Use.NOTHING, decide(candidate = null, good = null, failures = 0))
    }

    @Test
    fun `with only a known good plan that one is used`() {
        assertEquals(PlanChoice.Use.KNOWN_GOOD, decide(candidate = null, good = 4, failures = 0))
    }

    /**
     * The trap the Business Owner named: a plan issued to escape a block must
     * not be pushed aside by our own safety net.
     *
     * Node A is blocked. A plan naming node B is issued. Two bad moments on a
     * train roll the device back to the plan it remembers as good, which is the
     * blocked one. If "known good" were permanent the device would sit there
     * for ever, defeated by the mechanism meant to protect it.
     *
     * What breaks the trap is that a known good plan which stops working is
     * forgotten - and then the newest plan is back in play.
     */
    @Test
    fun `a known good plan that stops working stops being preferred`() {
        assertEquals(
            "rolled back to the plan on the blocked node",
            PlanChoice.Use.KNOWN_GOOD,
            decide(candidate = 5, good = 4, failures = 2),
        )

        // It failed too, so the store forgets it: no known good, no failures.
        assertEquals(
            "the newest plan is back in play once the old one is forgotten",
            PlanChoice.Use.CANDIDATE,
            decide(candidate = 5, good = null, failures = 0),
        )
    }

    // ---- When to stop trying -------------------------------------------
    //
    // The cases below are the ones the service used to get wrong, and each of
    // them is a restatement of a test above. That is the point: the rule about
    // which plan to use was right and tested, the rule about when to give up
    // lived in the service, and it assumed the first would eventually answer
    // KNOWN_GOOD. Two of the tests above say plainly that it will not.

    private fun retry(candidate: Long?, good: Long?, failures: Int, justFailed: PlanChoice.Use) =
        PlanChoice.worthRetrying(
            candidateSeq = candidate,
            goodSeq = good,
            failures = failures,
            limit = 2,
            justFailed = justFailed,
        )

    @Test
    fun `a first failure is worth another go`() {
        assertTrue(retry(candidate = 5, good = 4, failures = 1, justFailed = PlanChoice.Use.CANDIDATE))
    }

    @Test
    fun `a rollback is always worth taking`() {
        // The candidate has spent its attempts and there is an older plan that
        // once worked. This is the case the whole rollback exists for.
        assertTrue(retry(candidate = 5, good = 4, failures = 2, justFailed = PlanChoice.Use.CANDIDATE))
    }

    /**
     * The loop, stated as the two shapes that produced it.
     *
     * A device on 23 September sat in exactly this: its node had become
     * unreachable from its network, the proof failed every time, and the
     * service rebuilt the tunnel every few seconds for as long as the
     * application was open, saying "reconnecting" at each turn. Pressing OFF
     * stopped what was running and the next rebuild was already queued behind
     * it.
     */
    @Test
    fun `with nothing else to try the attempts run out`() {
        // Nothing has ever been proved, so there is no fallback and never
        // will be one until something works.
        assertTrue(
            "the second attempt is still owed",
            retry(candidate = 5, good = null, failures = 1, justFailed = PlanChoice.Use.CANDIDATE),
        )
        assertFalse(
            "and after it there is nothing different left to try",
            retry(candidate = 5, good = null, failures = 2, justFailed = PlanChoice.Use.CANDIDATE),
        )

        // The candidate was proved once, so the plan known to work IS the
        // candidate. Rolling back to it would be rolling back to itself.
        assertFalse(
            retry(candidate = 5, good = 5, failures = 2, justFailed = PlanChoice.Use.CANDIDATE),
        )
    }

    @Test
    fun `a stale failure count does not buy extra attempts`() {
        // Failures survive the process, so a device that has already spent
        // them does not get them back by being restarted.
        assertFalse(retry(candidate = 5, good = null, failures = 9, justFailed = PlanChoice.Use.CANDIDATE))
    }

    @Test
    fun `when the last plan that worked fails too there is nowhere left`() {
        // Reached only after the candidate has spent its attempts getting
        // here, and the store forgets the known good plan on the way - so the
        // candidate looking available again is not an invitation to try it a
        // third and fourth time.
        assertFalse(retry(candidate = 5, good = 4, failures = 2, justFailed = PlanChoice.Use.KNOWN_GOOD))
        assertFalse(retry(candidate = 5, good = null, failures = 0, justFailed = PlanChoice.Use.KNOWN_GOOD))
    }

    @Test
    fun `nothing stored is nothing to try`() {
        assertFalse(retry(candidate = null, good = null, failures = 0, justFailed = PlanChoice.Use.NOTHING))
        assertFalse(retry(candidate = null, good = null, failures = 0, justFailed = PlanChoice.Use.CANDIDATE))
    }

    /**
     * The whole run, counted.
     *
     * The number of attempts one press of the button can cost is the thing
     * that was unbounded, so it is asserted as a number rather than described.
     */
    @Test
    fun `one press cannot cost more than three attempts`() {
        var attempts = 0
        var candidate: Long? = 5
        var good: Long? = 4
        var failures = 0
        var using = PlanChoice.decide(candidate, good, failures, 2)

        while (attempts < 50) {
            attempts++
            // Every attempt fails: the node is unreachable from this network.
            when (using) {
                PlanChoice.Use.CANDIDATE -> failures++
                // The store forgets a known good plan that fails, and clears
                // the count with it.
                PlanChoice.Use.KNOWN_GOOD -> { good = null; failures = 0 }
                PlanChoice.Use.NOTHING -> Unit
            }
            if (!PlanChoice.worthRetrying(candidate, good, failures, 2, using)) break
            using = PlanChoice.decide(candidate, good, failures, 2)
        }

        assertEquals("candidate, candidate, then the plan that once worked", 3, attempts)

        // And with no fallback at all, two.
        attempts = 0
        candidate = 5
        good = null
        failures = 0
        using = PlanChoice.decide(candidate, good, failures, 2)
        while (attempts < 50) {
            attempts++
            if (using == PlanChoice.Use.CANDIDATE) failures++
            if (!PlanChoice.worthRetrying(candidate, good, failures, 2, using)) break
            using = PlanChoice.decide(candidate, good, failures, 2)
        }
        assertEquals("the same plan twice and no more", 2, attempts)
    }
}
