package download.simplevpn.config

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Everything that leaves the device for the tunnel is encrypted.
 *
 * The property was true before this file existed and nothing failed when it
 * stopped being true, because nothing checked it. "allowInsecure" carried the
 * comment "Never true"; a comment is not a check, and the one thing standing
 * between a working tunnel and a readable one was a person remembering.
 *
 * These read the config the engine is actually given, rather than the fields
 * the builder happens to expose. A transport added later that forgets TLS
 * fails here, which is the whole point: this is a property of the output, not
 * of any one branch.
 */
class EncryptedTransportTest {

    private val wsProfile = ConnectionProfile(
        alias = "node",
        host = "203.0.113.7",
        port = 443,
        transport = TransportParams.VlessWsTls(
            credentialUuid = "00000000-0000-4000-8000-000000000001",
            path = "/tunnel",
            serverName = "cover.example",
            hostHeader = "cover.example",
            fingerprint = "chrome",
        ),
    )

    private val realityProfile = ConnectionProfile(
        alias = "standby",
        host = "203.0.113.7",
        port = 8443,
        transport = TransportParams.VlessReality(
            credentialUuid = "00000000-0000-4000-8000-000000000002",
            flow = "xtls-rprx-vision",
            serverName = "www.example.com",
            publicKey = "key",
            shortId = "ab",
            fingerprint = "chrome",
        ),
    )

    private val policy = RoutingPolicy(
        directApps = emptyList(),
        directDomains = listOf("domain:gosuslugi.ru"),
        directIPs = emptyList(),
        proxyDomains = emptyList(),
        proxyIPs = emptyList(),
        russiaDirect = true,
    )

    private fun configFor(profile: ConnectionProfile): JSONObject =
        JSONObject(XrayConfigBuilder.build(profile, policy))

    private fun proxyOutbound(config: JSONObject): JSONObject {
        val outbounds = config.getJSONArray("outbounds")
        for (i in 0 until outbounds.length()) {
            val outbound = outbounds.getJSONObject(i)
            if (outbound.optString("tag") == "proxy") return outbound
        }
        throw AssertionError("there is no outbound carrying the tunnel")
    }

    @Test
    fun `the tunnel is never carried in the clear`() {
        // VLESS has no encryption of its own - "decryption": "none" is required
        // by the protocol - so everything depends on the transport underneath.
        // A stream with security "none" is a VPN that publishes what it
        // carries, and it would connect and work exactly like a good one.
        for (profile in listOf(wsProfile, realityProfile)) {
            val stream = proxyOutbound(configFor(profile)).getJSONObject("streamSettings")
            val security = stream.optString("security")
            assertTrue(
                "transport ${profile.transport.kind} carries the tunnel with security=$security",
                security == "tls" || security == "reality",
            )
        }
    }

    @Test
    fun `an invalid certificate is refused`() {
        // Accepting one turns an interception somebody could detect into one
        // nobody can. It is a single boolean between the two.
        val stream = proxyOutbound(configFor(wsProfile)).getJSONObject("streamSettings")
        val tls = stream.getJSONObject("tlsSettings")
        assertFalse(
            "the tunnel would accept any certificate presented to it",
            tls.optBoolean("allowInsecure", false),
        )
        assertEquals(
            "the certificate is not checked against a name",
            "cover.example",
            tls.optString("serverName"),
        )
    }

    @Test
    fun `the resolvers that would reveal every site are inside the tunnel`() {
        // Their exposure is the browsing history itself. The rule that keeps
        // them in the tunnel is written explicitly rather than left to the
        // default, and this is what stops a later rule from quietly taking
        // them out of it.
        val rules = configFor(wsProfile).getJSONObject("routing").getJSONArray("rules")
        var found = false
        for (i in 0 until rules.length()) {
            val rule = rules.getJSONObject(i)
            val ips = rule.optJSONArray("ip") ?: continue
            val addresses = (0 until ips.length()).map { ips.getString(it) }
            if (addresses.containsAll(listOf("1.1.1.1", "8.8.8.8"))) {
                assertEquals(
                    "the remote resolvers are routed out of the tunnel",
                    "proxy",
                    rule.optString("outboundTag"),
                )
                found = true
            }
        }
        assertTrue("nothing keeps the remote resolvers inside the tunnel", found)
    }

    @Test
    fun `the resolvers used for everything are encrypted ones`() {
        // Not merely inside the tunnel: encrypted at the resolver too, so that
        // the exit node is not handed the list either.
        val servers = configFor(wsProfile).getJSONObject("dns").getJSONArray("servers")
        var encrypted = 0
        for (i in 0 until servers.length()) {
            val entry = servers.opt(i)
            if (entry is String && entry.startsWith("https://")) encrypted++
        }
        assertTrue("the general-purpose resolvers are not encrypted", encrypted >= 2)
    }

    @Test
    fun `quic is refused rather than carried unencrypted`() {
        // A UDP escape hatch beside a TCP tunnel is traffic leaving on a path
        // nothing above has decided about.
        val rules = configFor(wsProfile).getJSONObject("routing").getJSONArray("rules")
        var blocked = false
        for (i in 0 until rules.length()) {
            val rule = rules.getJSONObject(i)
            if (rule.optString("network") == "udp" &&
                rule.optInt("port") == 443 &&
                rule.optString("outboundTag") == "block"
            ) {
                blocked = true
            }
        }
        assertTrue("QUIC is not refused, so traffic can leave beside the tunnel", blocked)
    }

    @Test
    fun `nothing listens beyond the device itself`() {
        // Both inbounds are the device talking to itself. One bound to any
        // address turns the phone into an open proxy for whoever shares its
        // network.
        val inbounds = configFor(wsProfile).getJSONArray("inbounds")
        assertTrue("no inbounds at all", inbounds.length() > 0)
        for (i in 0 until inbounds.length()) {
            val inbound = inbounds.getJSONObject(i)
            assertEquals(
                "inbound ${inbound.optString("tag")} listens beyond this device",
                "127.0.0.1",
                inbound.optString("listen"),
            )
        }
    }
}
