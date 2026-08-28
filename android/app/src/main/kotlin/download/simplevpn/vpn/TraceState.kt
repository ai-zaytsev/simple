package download.simplevpn.vpn

/**
 * Whether the detailed recording is off, running, or waiting to be sent.
 *
 * Separate from [VpnConnectionState] because it is a separate question with a
 * separate lifetime: a recording can be waiting to be sent long after the
 * tunnel has been stopped, and the tunnel runs perfectly well with no
 * recording at all. Folding the two together would mean every connection state
 * gained a variant, and the one that matters here - "there is a file on this
 * phone listing where you went" - would be the easiest to lose.
 */
sealed interface TraceState {

    /** Nothing is being recorded and nothing is waiting. */
    data object Idle : TraceState

    /**
     * A recording is running and will stop itself.
     *
     * [stopsAtElapsedMillis] is on the elapsed-realtime clock rather than the
     * wall clock, so that changing the time zone or the date does not extend a
     * recording or end it early.
     */
    data class Recording(val stopsAtElapsedMillis: Long) : TraceState

    /** A recording has finished and is on the device, waiting to be sent. */
    data object Ready : TraceState
}
