package download.simplevpn.config

import org.json.JSONObject

/**
 * Which traffic bypasses the tunnel and which is forced through it.
 *
 * Nobody chooses any of this. The person installs the application and presses
 * one button; every decision below is made by the server and arrives signed,
 * so a route that turns out wrong is corrected by an operator in a minute
 * rather than by a release in a week.
 *
 * The order the rules are applied in is the product rule, and it lives in code
 * rather than in the data. Leaving it to data would let one careless row put
 * "everything Russian goes direct" above the explicit lists, and the explicit
 * lists exist precisely for the cases where that is wrong.
 *
 *  1. [directApps] - excluded from the tunnel by the operating system.
 *  2. [directDomains] and [directIPs] - straight out, whatever else matches.
 *  3. [proxyDomains] and [proxyIPs] - through the tunnel, whatever else matches.
 *  4. Russian addresses and names - straight out, when [russiaDirect].
 *  5. Everything else - through the tunnel.
 *
 * Applications are a separate layer because Android offers exactly one
 * per-application control: an application is either inside the tunnel or
 * outside it. That is what banking applications need, since they judge by the
 * address they are seen from rather than the names they talk to. The reverse -
 * forcing one application through the tunnel while others go direct - has no
 * equivalent, because once traffic is inside the tunnel nothing distinguishes
 * which application produced it. That case is served by naming the domains.
 */
data class RoutingPolicy(
    val directApps: List<String>,
    val directDomains: List<String>,
    val directIPs: List<String>,
    val proxyDomains: List<String>,
    val proxyIPs: List<String>,
    val russiaDirect: Boolean,
) {

    /** Addresses that go straight out, including the local network. */
    val directIPRules: List<String>
        get() = buildList {
            // The local network always, and never from the server. A printer,
            // a router page or a file share on the same Wi-Fi has no business
            // crossing a tunnel to another country, and no rule from anywhere
            // should be able to make it.
            add(PRIVATE_RANGES)
            addAll(directIPs)
        }

    /** Russian addresses, applied after every explicit rule. */
    val russiaIPRules: List<String>
        get() = if (russiaDirect) listOf(RUSSIA_ADDRESSES) else emptyList()

    companion object {

        const val PRIVATE_RANGES = "geoip:private"
        const val RUSSIA_ADDRESSES = "geoip:ru"

        /**
         * What to use before the first plan arrives, and only then.
         *
         * Deliberately almost empty. A guessed list that lives in the build is
         * exactly the thing this stage exists to remove: it cannot be
         * corrected, and its presence makes a stale build look like a working
         * one. Keeping the local network is different - that rule can never be
         * wrong, and losing it would break somebody's printer.
         */
        val UNTIL_A_PLAN_ARRIVES = RoutingPolicy(
            directApps = emptyList(),
            directDomains = emptyList(),
            directIPs = emptyList(),
            proxyDomains = emptyList(),
            proxyIPs = emptyList(),
            russiaDirect = true,
        )

        /**
         * Reads the routing section of a plan.
         *
         * A missing section is not an error and not a reason to refuse the
         * plan: it is a plan from a server that does not send routing yet, and
         * the answer is to route as the build would have on its own.
         */
        fun parse(routing: JSONObject?): RoutingPolicy {
            if (routing == null) return UNTIL_A_PLAN_ARRIVES
            return RoutingPolicy(
                directApps = strings(routing, "direct_apps"),
                directDomains = strings(routing, "direct_domains"),
                directIPs = strings(routing, "direct_ips"),
                proxyDomains = strings(routing, "proxy_domains"),
                proxyIPs = strings(routing, "proxy_ips"),
                russiaDirect = routing.optBoolean("russia_direct", true),
            )
        }

        private fun strings(json: JSONObject, key: String): List<String> {
            val array = json.optJSONArray(key) ?: return emptyList()
            return buildList {
                for (i in 0 until array.length()) {
                    val value = array.optString(i, "")
                    if (value.isNotBlank()) add(value)
                }
            }
        }
    }
}
