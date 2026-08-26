package download.simplevpn.plan

import org.junit.Assert.assertEquals
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
}
