package download.simplevpn.vpn

/**
 * State shown to the user and driven by the service.
 *
 * The set is deliberately small and never contains an optimistic value: there
 * is no state that means "probably connected". Reporting a tunnel that does not
 * exist is treated as a defect, because a user who believes they are protected
 * and is not is worse off than one who sees an error.
 */
sealed interface VpnConnectionState {

    data object Disconnected : VpnConnectionState

    /** Asking for the system VPN consent, or building configuration. */
    data object Preparing : VpnConnectionState

    data object Connecting : VpnConnectionState

    data class Connected(val establishedAtMillis: Long) : VpnConnectionState

    /**
     * The tunnel is being re-established, for a reason this does not carry.
     *
     * It used to say "the underlying network changed", and so did the label on
     * the screen, and both were guesses. A network change is one of five ways
     * here; the others are a node that stopped answering, a failover to a
     * reserve, a plan being abandoned, and a recording being switched on.
     * Naming one of them cost a support conversation that went looking at the
     * network while the plan was the thing failing.
     */
    data object Reconnecting : VpnConnectionState

    data object Disconnecting : VpnConnectionState

    /**
     * Terminal for this attempt. [reason] is shown to the user as-is, so it must
     * describe what happened rather than name an internal symbol.
     */
    data class Failed(val reason: String) : VpnConnectionState

    val isActive: Boolean
        get() = this is Connecting || this is Connected || this is Reconnecting
}
