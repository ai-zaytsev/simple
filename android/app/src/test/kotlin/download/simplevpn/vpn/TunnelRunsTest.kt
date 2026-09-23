package download.simplevpn.vpn

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What OFF has to mean.
 *
 * The fault these describe was reported as "it does not react to OFF, and it
 * reconnects by itself". Both halves were the same thing: establishing a
 * tunnel is spread across three threads and two queues, and pressing OFF tore
 * down what existed at that instant while the work that builds a tunnel was
 * still on its way. It arrived a second later and built one.
 */
class TunnelRunsTest {

    @Test
    fun `work belonging to the run in progress goes on`() {
        val runs = TunnelRuns()
        val token = runs.begin()
        assertTrue(runs.isCurrent(token))
    }

    @Test
    fun `work queued before a stop does not run after it`() {
        val runs = TunnelRuns()
        val token = runs.begin()

        // The rebuild after a failed plan, the restart a network change
        // schedules, the proof that has just come back - all of them hold a
        // token from before this moment.
        runs.stop()

        assertFalse("a stop is the end of the run, not of one step of it", runs.isCurrent(token))
    }

    @Test
    fun `a rebuild inside the same run is still the same press of the button`() {
        val runs = TunnelRuns()
        val token = runs.begin()

        // Abandoning a plan that carries nothing and building again on another
        // one must not look like a stop: nobody asked for it to stop.
        assertTrue(runs.isCurrent(token))
    }

    @Test
    fun `a second press does not adopt the attempt the first one left behind`() {
        val runs = TunnelRuns()
        val first = runs.begin()
        runs.stop()
        val second = runs.begin()

        assertFalse(
            "the abandoned attempt must not carry on inside the new one",
            runs.isCurrent(first),
        )
        assertTrue(runs.isCurrent(second))
    }

    @Test
    fun `stopping twice leaves nothing current`() {
        val runs = TunnelRuns()
        val token = runs.begin()
        runs.stop()
        runs.stop()
        assertFalse(runs.isCurrent(token))
    }

    /**
     * A counter rather than a flag, and this is why.
     *
     * With a boolean, the second press would set "wanted" back to true and the
     * first press's half-finished attempt - still sitting in the queue - would
     * read it as permission to carry on. Two attempts would then establish
     * over each other.
     */
    @Test
    fun `an old attempt is not revived by a new press`() {
        val runs = TunnelRuns()
        val first = runs.begin()
        runs.stop()
        runs.begin()
        assertFalse(runs.isCurrent(first))
    }
}
