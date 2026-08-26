package download.simplevpn.config

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What the plan says about routing, and what the client makes of it.
 *
 * Worth testing here because a routing mistake is quiet: traffic goes the wrong
 * way and everything still appears to work, until somebody's bank refuses them
 * or a foreign service sees a Russian address. Nothing about that shows up as a
 * failure on a screen.
 */
class RoutingPolicyTest {

    private fun routing(json: String) = RoutingPolicy.parse(JSONObject(json))

    @Test
    fun `a plan without routing routes as the build would alone`() {
        val policy = RoutingPolicy.parse(null)
        assertEquals(RoutingPolicy.UNTIL_A_PLAN_ARRIVES, policy)
    }

    @Test
    fun `lists arrive as the server sent them`() {
        val policy = routing(
            """
            {"direct_apps":["ru.sberbankmobile"],
             "direct_domains":["domain:nalog.ru"],
             "direct_ips":["1.2.3.0/24"],
             "proxy_domains":["domain:chase.com"],
             "proxy_ips":["5.6.7.0/24"],
             "russia_direct":true}
            """.trimIndent(),
        )
        assertEquals(listOf("ru.sberbankmobile"), policy.directApps)
        assertEquals(listOf("domain:nalog.ru"), policy.directDomains)
        assertEquals(listOf("domain:chase.com"), policy.proxyDomains)
        assertEquals(listOf("5.6.7.0/24"), policy.proxyIPs)
        assertTrue(policy.russiaDirect)
    }

    @Test
    fun `the local network is always direct and never comes from the server`() {
        // A printer or a router page on the same Wi-Fi has no business crossing
        // a tunnel to another country, and no rule from anywhere should be able
        // to send it there.
        val policy = routing("""{"direct_ips":[]}""")
        assertTrue(policy.directIPRules.contains(RoutingPolicy.PRIVATE_RANGES))
    }

    @Test
    fun `the local network stays first even when the server sends addresses`() {
        val policy = routing("""{"direct_ips":["1.2.3.0/24"]}""")
        assertEquals(RoutingPolicy.PRIVATE_RANGES, policy.directIPRules.first())
        assertTrue(policy.directIPRules.contains("1.2.3.0/24"))
    }

    @Test
    fun `Russian addresses go direct by default`() {
        val policy = routing("""{}""")
        assertTrue(policy.russiaDirect)
        assertEquals(listOf(RoutingPolicy.RUSSIA_ADDRESSES), policy.russiaIPRules)
    }

    @Test
    fun `the server can switch off routing Russian addresses direct`() {
        // The day this serves somebody outside Russia, that rule is wrong. It
        // must be a switch rather than an assumption, and switching it must not
        // need a release.
        val policy = routing("""{"russia_direct":false}""")
        assertFalse(policy.russiaDirect)
        assertTrue(policy.russiaIPRules.isEmpty())
    }

    @Test
    fun `blank entries are dropped rather than passed to the engine`() {
        // A blank entry in a routing list is not harmless: engines differ on
        // whether an empty pattern matches nothing or everything, and finding
        // out which on somebody's phone is not the way.
        val policy = routing("""{"direct_domains":["domain:nalog.ru","","  "]}""")
        assertEquals(listOf("domain:nalog.ru"), policy.directDomains)
    }

    @Test
    fun `a list of the wrong shape is treated as absent`() {
        val policy = routing("""{"direct_domains":"not a list"}""")
        assertTrue(policy.directDomains.isEmpty())
    }
}
