package download.simplevpn.metrics

import org.json.JSONArray
import org.json.JSONObject

/**
 * What this device tells us about reaching the service.
 *
 * Counters and nothing else. There is no field here for an address the person
 * visited, a name they looked up, or anything they did through the tunnel -
 * only whether our own service could be reached, how long that took, and how
 * long the connection lasted.
 *
 * The reason the device reports at all is that it stands where the interesting
 * failures happen. A domain we can reach from our own machines and a phone in
 * Russia cannot is not a broken server, and nothing outside the country can
 * tell the difference. That measurement needs this vantage point, and this is
 * the smallest thing that provides it.
 *
 * Held in memory and sent as a sum. A report is a handful of totals covering
 * everything since the last one, so there is no per-attempt record to keep, to
 * lose, or to be asked for.
 */
object ServiceReport {

    private val lock = Any()

    private var attempts = 0
    private var successes = 0
    private var reconnects = 0
    private var sessionSeconds = 0L
    private var latencySumMs = 0L
    private var latencySamples = 0
    private var nodeAlias = ""
    private var entryKind = ""

    private data class Probe(val target: String, val ok: Boolean, val latencyMs: Int?)

    private val probes = ArrayDeque<Probe>()

    /**
     * How many probe outcomes to carry between reports.
     *
     * Bounded because a device that cannot reach the service is also a device
     * that cannot deliver the news, and the queue would otherwise grow for as
     * long as the trouble lasts - which is exactly when the phone can least
     * afford it.
     */
    private const val MAX_PROBES = 40

    fun attempted(node: String, entry: String) = synchronized(lock) {
        attempts++
        if (node.isNotBlank()) nodeAlias = node
        if (entry.isNotBlank()) entryKind = entry
    }

    fun connected(latencyMs: Long?) = synchronized(lock) {
        successes++
        if (latencyMs != null && latencyMs in 1..60_000) {
            latencySumMs += latencyMs
            latencySamples++
        }
    }

    fun reconnected() = synchronized(lock) { reconnects++ }

    fun sessionEnded(seconds: Long) = synchronized(lock) {
        if (seconds > 0) sessionSeconds += seconds
    }

    /**
     * Records what one of our own addresses did when it was tried.
     *
     * Only ever called with an address from our own connection plan or signed
     * bootstrap descriptor. The server independently checks the current node
     * and bootstrap tables and drops anything else, so a modified build cannot
     * turn this into a way of reporting where somebody went.
     */
    fun probed(target: String, ok: Boolean, latencyMs: Int?) = synchronized(lock) {
        if (target.isBlank()) return
        if (probes.size >= MAX_PROBES) probes.removeFirst()
        probes.addLast(Probe(target, ok, latencyMs))
    }

    /** True when there is anything worth spending a request on. */
    fun worthSending(): Boolean = synchronized(lock) {
        attempts > 0 || reconnects > 0 || sessionSeconds > 0 || probes.isNotEmpty()
    }

    /**
     * Takes everything gathered so far and clears it.
     *
     * Cleared on the way out rather than after a successful send: a report
     * that fails to arrive is a lost window, and holding it for a retry means
     * holding a growing pile on a phone that is already having trouble. The
     * numbers are approximate by design and one missing window does not change
     * what they show.
     */
    fun drain(): String = synchronized(lock) {
        val body = JSONObject()
            .put("node_alias", nodeAlias)
            .put("entry_kind", entryKind)
            .put("attempts", attempts)
            .put("successes", successes)
            .put("reconnects", reconnects)
            .put("session_seconds", sessionSeconds)
            .put("latency_ms_sum", latencySumMs)
            .put("latency_samples", latencySamples)

        val list = JSONArray()
        for (probe in probes) {
            val one = JSONObject()
                .put("target", probe.target)
                .put("ok", probe.ok)
            if (probe.latencyMs != null) one.put("latency_ms", probe.latencyMs)
            list.put(one)
        }
        body.put("probes", list)

        attempts = 0
        successes = 0
        reconnects = 0
        sessionSeconds = 0
        latencySumMs = 0
        latencySamples = 0
        probes.clear()

        body.toString()
    }
}
