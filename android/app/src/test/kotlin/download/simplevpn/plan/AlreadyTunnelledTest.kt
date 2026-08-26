package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.TransportParams
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Noticing that the network already runs through one of our own nodes.
 *
 * Tested here because the situation it exists for - somebody running this VPN
 * on their router - cannot be produced on demand, and the decision must be
 * right the first time it happens on somebody's phone.
 */
class AlreadyTunnelledTest {

    private fun node(alias: String, host: String) = ConnectionProfile(
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

    private val nodes = listOf(node("n-1", "10.0.0.1"), node("n-2", "10.0.0.2"))

    @Test
    fun `an ordinary address means there is no tunnel in the way`() {
        val verdict = AlreadyTunnelled.decide("203.0.113.9", nodes)
        assertEquals(AlreadyTunnelled.Verdict.NotTunnelled, verdict)
    }

    @Test
    fun `being seen from our own node means the network already goes through us`() {
        val verdict = AlreadyTunnelled.decide("10.0.0.2", nodes)
        assertEquals(AlreadyTunnelled.Verdict.ThroughOurNode("n-2"), verdict)
    }

    @Test
    fun `a reserve counts as much as the primary`() {
        // A router might be pointed at any node we run, not only the one this
        // phone would have chosen.
        val verdict = AlreadyTunnelled.decide("10.0.0.1", nodes)
        assertTrue(verdict is AlreadyTunnelled.Verdict.ThroughOurNode)
    }

    @Test
    fun `not knowing is not the same as knowing there is no tunnel`() {
        // A network that cannot reach the Control Plane says nothing about
        // routers. Refusing to connect on silence would turn an unanswered
        // question into an outage.
        assertEquals(AlreadyTunnelled.Verdict.Unknown, AlreadyTunnelled.decide(null, nodes))
        assertEquals(AlreadyTunnelled.Verdict.Unknown, AlreadyTunnelled.decide("", nodes))
    }

    @Test
    fun `a plan with no endpoints cannot match anything`() {
        assertEquals(
            AlreadyTunnelled.Verdict.NotTunnelled,
            AlreadyTunnelled.decide("10.0.0.1", emptyList()),
        )
    }
}
