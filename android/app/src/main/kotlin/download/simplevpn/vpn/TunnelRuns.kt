package download.simplevpn.vpn

import java.util.concurrent.atomic.AtomicInteger

/**
 * Which attempt at a tunnel is the current one.
 *
 * This is the whole of what makes OFF mean off.
 *
 * Establishing a tunnel is not one action. It asks the Control Plane where to
 * connect, probes endpoints, builds an interface, starts an engine, then goes
 * away to prove that traffic actually crosses it - and every one of those
 * steps hands work to another thread or to a queue. Pressing OFF used to tear
 * down what existed at that instant and nothing else, so whatever was already
 * queued ran afterwards and built the tunnel again. A person who switched the
 * connection off watched it come back on a second later, and pressing OFF
 * again queued another one.
 *
 * So a stop is not a teardown, it is the end of a run. Every piece of deferred
 * work remembers the run it belongs to and does nothing if that run is over.
 * A rebuild after a failed plan stays inside the same run, because nobody
 * asked for it to stop - it is the same press of the button, still being
 * answered.
 *
 * A counter rather than a flag, because START, STOP, START in quick succession
 * must leave the first start abandoned rather than adopted by the second.
 */
class TunnelRuns {

    private val current = AtomicInteger(0)

    /** Begins a run and returns its token. Whatever ran before is over. */
    fun begin(): Int = current.incrementAndGet()

    /** Ends whatever is running. Nothing queued before this point survives. */
    fun stop() {
        current.incrementAndGet()
    }

    /** Whether work belonging to [token] should still go on. */
    fun isCurrent(token: Int): Boolean = current.get() == token
}
