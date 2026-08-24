package download.simplevpn.core

import vpncore.Vpncore

/**
 * Counters from the packet bridge, for the one question the user interface
 * cannot otherwise answer: the tunnel says connected, so where do the packets
 * stop?
 *
 * Every call goes to the native side rather than to a cached value, because a
 * counter that is not moving is exactly the observation being made.
 *
 * This disappears together with the vertical slice.
 */
object BridgeDiagnostics {

    fun snapshot(): String = try {
        Vpncore.bridgeStats()
    } catch (t: Throwable) {
        "counters unavailable"
    }
}
