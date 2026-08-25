package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.TransportParams
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Failover, which is only ever exercised when something is already broken.
 *
 * That is exactly why it is tested here. The path runs on a phone whose node
 * has just died, which is the worst possible moment to discover that reserves
 * are tried in the wrong order or that the client never returns to a primary
 * that recovered.
 */
class EndpointChoiceTest {

    private fun endpoint(alias: String, host: String) = ConnectionProfile(
        alias = alias,
        host = host,
        port = 443,
        transport = TransportParams.VlessWsTls(
            credentialUuid = "00000000-0000-4000-8000-000000000000",
            path = "/x",
            serverName = "example.invalid",
            hostHeader = "example.invalid",
            fingerprint = "chrome",
        ),
    )

    private val primary = endpoint("n-1", "10.0.0.1")
    private val first = endpoint("n-2", "10.0.0.2")
    private val second = endpoint("n-3", "10.0.0.3")
    private val all = listOf(primary, first, second)

    @Test
    fun `a working primary is used`() {
        val choice = EndpointChoice.choose(all) { true }!!
        assertEquals(primary, choice.endpoint)
        assertEquals(0, choice.index)
        assertTrue(choice.probed)
    }

    @Test
    fun `a dead primary moves to the first reserve`() {
        val choice = EndpointChoice.choose(all) { it != primary }!!
        assertEquals(first, choice.endpoint)
        assertEquals(1, choice.index)
    }

    @Test
    fun `reserves are tried in the order the server gave`() {
        val choice = EndpointChoice.choose(all) { it == second }!!
        assertEquals(second, choice.endpoint)
        assertEquals(2, choice.index)
    }

    @Test
    fun `when nothing answers the primary is used anyway`() {
        // A probe is a plain TCP connect. A network that blocks it may still
        // carry the tunnel, so refusing to connect would turn a strict check
        // into an outage of our own making.
        val choice = EndpointChoice.choose(all) { false }!!
        assertEquals(primary, choice.endpoint)
        assertFalse(choice.probed)
    }

    @Test
    fun `an empty plan chooses nothing`() {
        assertNull(EndpointChoice.choose(emptyList()) { true })
    }

    @Test
    fun `a single failure is not enough to move`() {
        // A node that fails once is a network having a bad moment. Moving off
        // it would trade a working connection for a reconnection.
        val failures = EndpointChoice.Failures(threshold = 2)
        assertFalse(failures.failed())
    }

    @Test
    fun `enough failures in a row mean the node is gone`() {
        val failures = EndpointChoice.Failures(threshold = 2)
        assertFalse(failures.failed())
        assertTrue(failures.failed())
    }

    @Test
    fun `a success resets the count`() {
        val failures = EndpointChoice.Failures(threshold = 3)
        failures.failed()
        failures.failed()
        failures.succeeded()
        assertFalse(failures.failed())
        assertEquals(1, failures.count)
    }

    @Test
    fun `a threshold below one still takes one failure`() {
        // The number comes from the server. A zero would otherwise mean the
        // endpoint is abandoned before it has failed at all.
        val failures = EndpointChoice.Failures(threshold = 0)
        assertTrue(failures.failed())
    }

    @Test
    fun `the next endpoint follows the current one`() {
        assertEquals(first, EndpointChoice.next(all, after = 0))
        assertEquals(second, EndpointChoice.next(all, after = 1))
    }

    @Test
    fun `after the last endpoint it returns to the primary`() {
        // The primary may well have recovered by now, and running off the end
        // of the list would leave somebody disconnected beside a node that
        // works.
        assertEquals(primary, EndpointChoice.next(all, after = 2))
    }

    @Test
    fun `a plan with one endpoint has nowhere to go`() {
        assertEquals(primary, EndpointChoice.next(listOf(primary), after = 0))
    }
}
