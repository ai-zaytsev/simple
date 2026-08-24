package download.simplevpn.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import download.simplevpn.MainActivity
import download.simplevpn.R
import download.simplevpn.config.RoutingPolicy
import download.simplevpn.config.SliceProfileSource
import download.simplevpn.config.XrayConfigBuilder
import download.simplevpn.core.EngineStartResult
import download.simplevpn.core.LibXrayEngine
import download.simplevpn.core.TunBridge
import download.simplevpn.core.XrayEngine
import download.simplevpn.net.NetworkMonitor
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Owns the tunnel for the lifetime of the connection.
 *
 * The tunnel lives here and not in the Activity, so closing the screen leaves
 * it running. Everything the service does is wrapped so that a transport
 * failure ends the connection with a message instead of ending the process: an
 * engine that cannot start is an expected outcome, not a crash.
 */
class SimpleVpnService : VpnService() {

    private val engine: XrayEngine = LibXrayEngine(this)
    private val bridge = TunBridge()
    private var tunnel: ParcelFileDescriptor? = null

    /**
     * Once the descriptor is handed to the bridge it is no longer ours to
     * close: closing it on both sides closes it twice.
     */
    private var tunnelHandedOver = false
    private var networkMonitor: NetworkMonitor? = null
    private val starting = AtomicBoolean(false)

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> handleStart()
            ACTION_STOP -> handleStop(VpnConnectionState.Disconnected)
            else -> {
                // Restarted by the system with a null intent. Nothing is known
                // about the previous session, so the safe action is to stop
                // rather than to establish a tunnel the user did not ask for.
                Log.i(TAG, "restarted without an action, stopping")
                handleStop(VpnConnectionState.Disconnected)
            }
        }
        // START_NOT_STICKY: the system must not silently recreate a tunnel.
        // A VPN that comes back without the user asking is a surprise, and
        // surprises about traffic routing are not acceptable.
        return START_NOT_STICKY
    }

    private fun handleStart() {
        if (!starting.compareAndSet(false, true)) {
            Log.i(TAG, "start already in progress")
            return
        }

        try {
            startForeground(NOTIFICATION_ID, buildNotification(getString(R.string.status_connecting)))
            VpnController.update(VpnConnectionState.Connecting)

            val profileResult = SliceProfileSource.load(this)
            if (profileResult is SliceProfileSource.Result.Missing) {
                failAndStop(profileResult.reason)
                return
            }
            val profile = (profileResult as SliceProfileSource.Result.Available).profile

            val policy = RoutingPolicy.DEFAULT

            val descriptor = TunConfigurator(this).establish(policy)
            if (descriptor == null) {
                failAndStop(getString(R.string.error_tun_not_established))
                return
            }
            tunnel = descriptor

            val configJson = XrayConfigBuilder.build(profile, policy)

            when (val result = engine.start(configJson, descriptor.fd)) {
                is EngineStartResult.Started -> Unit

                is EngineStartResult.Unavailable -> {
                    failAndStop(result.reason)
                    return
                }

                is EngineStartResult.Failed -> {
                    Log.w(TAG, "engine failed to start", result.cause)
                    failAndStop(result.reason)
                    return
                }
            }

            // The engine only listens on loopback. Until the bridge is running,
            // not a single packet from the device reaches it, so the tunnel is
            // not established until this succeeds.
            val rawFd = descriptor.detachFd()
            tunnelHandedOver = true

            when (val bridged = bridge.start(rawFd, TunConfigurator.MTU, XrayConfigBuilder.SOCKS_PORT)) {
                is TunBridge.Result.Started -> {
                    startNetworkMonitor()
                    VpnController.update(VpnConnectionState.Connected(System.currentTimeMillis()))
                    updateNotification(getString(R.string.status_connected))
                }

                is TunBridge.Result.Unavailable -> failAndStop(bridged.reason)
                is TunBridge.Result.Failed -> failAndStop(bridged.reason)
            }
        } catch (t: Throwable) {
            // Deliberately broad. Anything thrown while establishing must end
            // as a reported failure: a crash here would take the process down
            // and leave the system VPN slot in an unclear state.
            Log.e(TAG, "unexpected failure while starting", t)
            failAndStop(getString(R.string.error_unexpected))
        } finally {
            starting.set(false)
        }
    }

    private fun restartEngineForNewNetwork() {
        if (!engine.isRunning) return
        VpnController.update(VpnConnectionState.Reconnecting)
        updateNotification(getString(R.string.status_reconnecting))

        try {
            // Only the engine restarts. The bridge already owns the interface
            // and talks to loopback, so the address it forwards to does not
            // change when the underlying network does. Rebuilding the interface
            // here would drop the tunnel the user is currently using.
            engine.stop()

            val profileResult = SliceProfileSource.load(this)
            if (profileResult !is SliceProfileSource.Result.Available) {
                failAndStop(getString(R.string.error_unexpected))
                return
            }

            val configJson = XrayConfigBuilder.build(profileResult.profile, RoutingPolicy.DEFAULT)
            when (val result = engine.start(configJson, TUN_FD_OWNED_BY_BRIDGE)) {
                is EngineStartResult.Started -> {
                    VpnController.update(VpnConnectionState.Connected(System.currentTimeMillis()))
                    updateNotification(getString(R.string.status_connected))
                }

                is EngineStartResult.Unavailable -> failAndStop(result.reason)
                is EngineStartResult.Failed -> failAndStop(result.reason)
            }
        } catch (t: Throwable) {
            Log.e(TAG, "failed to restart after network change", t)
            failAndStop(getString(R.string.error_unexpected))
        }
    }

    private fun startNetworkMonitor() {
        networkMonitor?.stop()
        networkMonitor = NetworkMonitor(
            context = this,
            onUnderlyingNetworkChanged = { restartEngineForNewNetwork() },
            onNetworkLost = {
                // Not a failure: the device may be between networks. The state
                // says reconnecting, and the next available network triggers a
                // restart.
                VpnController.update(VpnConnectionState.Reconnecting)
                updateNotification(getString(R.string.status_reconnecting))
            },
        ).also { it.start() }
    }

    private fun failAndStop(reason: String) {
        VpnController.update(VpnConnectionState.Failed(reason))
        teardown()
        stopSelf()
    }

    private fun handleStop(finalState: VpnConnectionState) {
        VpnController.update(VpnConnectionState.Disconnecting)
        teardown()
        VpnController.update(finalState)
        stopSelf()
    }

    private fun teardown() {
        networkMonitor?.stop()
        networkMonitor = null

        // Bridge first: it holds the interface, and stopping the engine
        // underneath it would leave packets arriving with nowhere to go.
        bridge.stop()

        try {
            engine.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "engine stop failed", t)
        }

        try {
            // Only when the bridge never took it. After hand-over the bridge
            // owns the descriptor and has already released it.
            if (!tunnelHandedOver) {
                tunnel?.close()
            }
        } catch (t: Throwable) {
            Log.w(TAG, "closing tunnel failed", t)
        } finally {
            tunnel = null
            tunnelHandedOver = false
        }

        stopForeground(STOP_FOREGROUND_REMOVE)
    }

    /** The user revoked consent, or another VPN application took over. */
    override fun onRevoke() {
        Log.i(TAG, "consent revoked")
        teardown()
        VpnController.update(VpnConnectionState.Disconnected)
        stopSelf()
        super.onRevoke()
    }

    override fun onDestroy() {
        teardown()
        if (VpnController.state.value.isActive) {
            VpnController.update(VpnConnectionState.Disconnected)
        }
        super.onDestroy()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        )
        val manager = getSystemService(NotificationManager::class.java)
        manager?.createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val open = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )

        // NotificationCompat rather than Notification.Builder: the latter needs
        // a channel from API 26 and the minimum supported level here is 24.
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_launcher)
            .setContentIntent(open)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(NotificationManager::class.java) ?: return
        try {
            manager.notify(NOTIFICATION_ID, buildNotification(text))
        } catch (t: Throwable) {
            // Notification permission may be absent. The tunnel does not depend
            // on the notification being visible, so this never fails a start.
            Log.w(TAG, "could not update notification", t)
        }
    }

    companion object {
        const val ACTION_START = "download.simplevpn.action.START"
        const val ACTION_STOP = "download.simplevpn.action.STOP"

        private const val TAG = "SimpleVpnService"

        /**
         * The engine never touches the interface: the bridge owns it. The
         * parameter stays on the interface because a future transport may need
         * it, and passing a descriptor the engine must not use would be worse
         * than passing none.
         */
        private const val TUN_FD_OWNED_BY_BRIDGE = 0
        private const val CHANNEL_ID = "vpn_status"
        private const val NOTIFICATION_ID = 1
    }
}
