package download.simplevpn.vpn

import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat
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
        // ContextCompat: startForegroundService exists from API 26 and the
        // minimum supported level here is 24.
        ContextCompat.startForegroundService(context, intent)
    }

    /**
     * Asks a running service to check now rather than at its next tick.
     *
     * Called when the application is brought to the screen. Somebody whose
     * connection has stopped working opens the application to find out why,
     * and that is exactly the moment an answer is worth having; waiting out
     * the remaining minutes of a timer would show them a screen that says
     * everything is fine.
     *
     * It costs nothing to run, because it runs only when somebody is looking.
     * Asking on a shorter timer instead would cost battery on every phone,
     * awake or not, for the sake of the rare minute when it matters.
     */
    fun recheck(context: Context) {
        if (state.value !is VpnConnectionState.Connected) return
        val intent = Intent(context, SimpleVpnService::class.java).apply {
            action = SimpleVpnService.ACTION_RECHECK
        }
        // Not startForegroundService: this must never bring a stopped service
        // to life, only prod one that is already running.
        context.startService(intent)
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
