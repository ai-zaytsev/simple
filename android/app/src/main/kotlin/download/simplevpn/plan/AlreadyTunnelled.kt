package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile

/**
 * Whether this phone's network already runs through one of our own nodes.
 *
 * That is what happens when somebody puts this VPN on their router. The phone
 * sees ordinary Wi-Fi and has no way to tell from the inside: a router's tunnel
 * leaves no trace on the device. What gives it away is the address the phone is
 * seen from - if that address is one of our nodes, the traffic is already
 * going through us.
 *
 * Building a second tunnel inside the first is wasteful at best. At worst the
 * phone's tunnel to a node travels through the router's tunnel to the same
 * node, and a stall in the outer stream stalls the inner one with it.
 *
 * The decision is a pure function of two lists so that it can be stated,
 * argued about and tested without a router, a phone or a network.
 */
object AlreadyTunnelled {

    /**
     * @param seenFrom the address the Control Plane says this device arrives
     *   from, or null when nothing could be asked
     * @param endpoints the nodes this plan offers
     */
    fun decide(seenFrom: String?, endpoints: List<ConnectionProfile>): Verdict {
        // Not knowing is not the same as knowing there is no tunnel. A network
        // that cannot reach the Control Plane says nothing about routers, and
        // refusing to connect on silence would turn an unanswered question into
        // an outage.
        if (seenFrom.isNullOrBlank()) return Verdict.Unknown

        val node = endpoints.firstOrNull { it.host == seenFrom }
            ?: return Verdict.NotTunnelled

        return Verdict.ThroughOurNode(node.alias)
    }

    sealed interface Verdict {
        /** Nothing in the way: build the tunnel. */
        data object NotTunnelled : Verdict

        /** Nothing could be asked. Proceed, because silence is not an answer. */
        data object Unknown : Verdict

        /** The network already runs through this node of ours. */
        data class ThroughOurNode(val alias: String) : Verdict
    }
}
