package download.simplevpn.vpn

/**
 * The one bounded window in which destinations may be written.
 *
 * Android's Handler owns the callback, but it must not own the decision. A
 * duplicate start used to remove that callback and then return because the
 * recording was already active, leaving the detailed log with no deadline.
 * This small state machine makes the decision explicit and testable: only the
 * first start opens a window, and every later start keeps its original end.
 */
internal class TraceWindow(private val limitMillis: Long) {

    init {
        require(limitMillis > 0) { "A trace window must have a positive limit" }
    }

    @Volatile
    private var deadline: Long? = null

    val isRunning: Boolean
        get() = deadline != null

    /** Starts once. A repeated request returns the original deadline. */
    @Synchronized
    fun start(nowElapsedMillis: Long): Start {
        val existing = deadline
        if (existing != null) return Start(existing, newlyStarted = false)

        val created = nowElapsedMillis + limitMillis
        deadline = created
        return Start(created, newlyStarted = true)
    }

    /** Stops once. False means there was no active destination recording. */
    @Synchronized
    fun stop(): Boolean {
        if (deadline == null) return false
        deadline = null
        return true
    }

    data class Start(
        val stopsAtElapsedMillis: Long,
        val newlyStarted: Boolean,
    )
}

