package download.simplevpn.vpn

import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat
import download.simplevpn.core.SessionLog
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

    private val _trace = MutableStateFlow<TraceState>(TraceState.Idle)

    /** Whether a detailed recording is running or waiting to be sent. */
    val trace: StateFlow<TraceState> = _trace.asStateFlow()

    internal fun updateTrace(next: TraceState) {
        _trace.value = next
    }

    /** Restores a finished recording after the Activity or process returns. */
    fun restoreTrace(context: Context) {
        _trace.value = restoredTraceState(_trace.value, SessionLog.hasTrace(context))
    }

    /**
     * Marks the recording gone, after it has been sent or deleted.
     *
     * Called by the screen rather than the service, because the screen is
     * where sending happens and the file must stop being offered the moment it
     * is no longer there.
     */
    fun traceCleared() {
        if (_trace.value is TraceState.Ready) _trace.value = TraceState.Idle
    }

    /**
     * Starts the detailed recording.
     *
     * Only ever reached from a deliberate tap, after the screen has said what
     * the recording holds. There is no other caller and there should not be
     * one: a recording that can begin without somebody being told is the
     * defect this whole feature replaces.
     */
    fun startTrace(context: Context) {
        if (state.value !is VpnConnectionState.Connected) return
        val intent = Intent(context, SimpleVpnService::class.java).apply {
            action = SimpleVpnService.ACTION_TRACE_START
        }
        context.startService(intent)
    }

    fun stopTrace(context: Context) {
        val intent = Intent(context, SimpleVpnService::class.java).apply {
            action = SimpleVpnService.ACTION_TRACE_STOP
        }
        context.startService(intent)
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

/** Keeps an active recording intact and otherwise follows what is on disk. */
internal fun restoredTraceState(current: TraceState, hasTrace: Boolean): TraceState =
    if (current is TraceState.Recording) {
        current
    } else if (hasTrace) {
        TraceState.Ready
    } else {
        TraceState.Idle
    }
