package download.simplevpn.vpn

import android.content.Context
import android.content.Intent
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Single source of truth for connection state, owned by the process rather than
 * by any Activity.
 *
 * The Activity observes this and may be destroyed at any moment without
 * affecting the tunnel. That is an acceptance criterion for this stage, and it
 * is the reason state does not live in a ViewModel.
 */
object VpnController {

    private val _state = MutableStateFlow<VpnConnectionState>(VpnConnectionState.Disconnected)
    val state: StateFlow<VpnConnectionState> = _state.asStateFlow()

    internal fun update(next: VpnConnectionState) {
        _state.value = next
    }

    fun start(context: Context) {
        val intent = Intent(context, SimpleVpnService::class.java).apply {
            action = SimpleVpnService.ACTION_START
        }
        context.startForegroundService(intent)
    }

    fun stop(context: Context) {
        val intent = Intent(context, SimpleVpnService::class.java).apply {
            action = SimpleVpnService.ACTION_STOP
        }
        // Not startForegroundService: a stop request must not resurrect a dead
        // service just to shut it down again.
        context.startService(intent)
    }
}
