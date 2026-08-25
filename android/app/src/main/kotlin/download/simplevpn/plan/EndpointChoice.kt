package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile

/**
 * Which endpoint to use, and when to stop using it.
 *
 * Both decisions are here, as functions of their inputs and nothing else. They
 * are the part of failover that can be got wrong quietly - trying reserves in
 * the wrong order, giving up on a node too early, never coming back to a node
 * that recovered - and none of that is visible from the outside until somebody
 * loses their connection.
 */
object EndpointChoice {

    /**
     * Picks the first endpoint that answers, in the order the server gave.
     *
     * Server order, always, and never a remembered favourite. The order is a
     * decision the server made about capacity and reach; a client that
     * preferred whatever worked last would keep a node in use after the server
     * had moved everybody off it, and would never return to a primary that
     * recovered.
     *
     * When nothing answers the primary is returned anyway. Refusing to connect
     * because a probe failed would turn one strict check into an outage: the
     * probe is a plain TCP connect, and a network that blocks it may still
     * carry the tunnel.
     */
    fun choose(
        endpoints: List<ConnectionProfile>,
        reachable: (ConnectionProfile) -> Boolean,
    ): Choice? {
        if (endpoints.isEmpty()) return null

        endpoints.forEachIndexed { index, endpoint ->
            if (reachable(endpoint)) {
                return Choice(endpoint, index, probed = true)
            }
        }
        return Choice(endpoints.first(), 0, probed = false)
    }

    /**
     * @param index position in the server's order, so a later failure can move
     *   on rather than start again at a node just found to be dead
     * @param probed whether the endpoint actually answered, or is the fallback
     */
    data class Choice(val endpoint: ConnectionProfile, val index: Int, val probed: Boolean)

    /**
     * Counts consecutive failures against the number the server chose.
     *
     * Consecutive, not total: a node that fails once an hour is a network
     * having a bad moment, and moving off it would trade a working connection
     * for a reconnection. A node that fails every check in a row is gone.
     */
    class Failures(private val threshold: Int) {
        private var consecutive = 0

        val count: Int get() = consecutive

        fun succeeded() {
            consecutive = 0
        }

        /** @return true when the endpoint has failed often enough to leave. */
        fun failed(): Boolean {
            consecutive += 1
            return consecutive >= threshold.coerceAtLeast(1)
        }
    }

    /**
     * The next endpoint to try after one has been abandoned.
     *
     * Wraps around, because the primary may well have recovered by the time
     * the last reserve has been tried, and a client that ran off the end of
     * the list would sit disconnected next to a node that works.
     */
    fun next(endpoints: List<ConnectionProfile>, after: Int): ConnectionProfile? {
        if (endpoints.isEmpty()) return null
        return endpoints[(after + 1) % endpoints.size]
    }
}
