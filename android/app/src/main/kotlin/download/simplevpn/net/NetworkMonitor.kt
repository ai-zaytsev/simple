package download.simplevpn.net

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.util.Log

/**
 * Reports when the underlying network changes, for example Wi-Fi to mobile.
 *
 * The TUN interface survives such a change, but sockets the engine opened over
 * the old network do not: they stay open and never deliver anything. Without
 * reacting, the tunnel looks established and passes no traffic, which is the
 * worst of both worlds. So the service restarts the engine on a real change.
 *
 * Only transport-level changes are reported. Capability updates fire constantly
 * and restarting on each would make the tunnel flap on a weak connection.
 */
class NetworkMonitor(
    context: Context,
    private val onUnderlyingNetworkChanged: (Network) -> Unit,
    private val onNetworkLost: () -> Unit,
) {

    private val connectivityManager =
        context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

    private var currentNetwork: Network? = null
    private var registered = false

    private val callback = object : ConnectivityManager.NetworkCallback() {

        override fun onAvailable(network: Network) {
            val previous = currentNetwork
            currentNetwork = network
            if (previous != null && previous != network) {
                Log.i(TAG, "underlying network changed")
                onUnderlyingNetworkChanged(network)
            }
        }

        override fun onLost(network: Network) {
            if (currentNetwork == network) {
                currentNetwork = null
                Log.i(TAG, "underlying network lost")
                onNetworkLost()
            }
        }
    }

    fun start() {
        if (registered) return
        val request = NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
            .build()
        try {
            connectivityManager.registerNetworkCallback(request, callback)
            registered = true
        } catch (t: Throwable) {
            Log.w(TAG, "could not register network callback", t)
        }
    }

    fun stop() {
        if (!registered) return
        try {
            connectivityManager.unregisterNetworkCallback(callback)
        } catch (t: Throwable) {
            Log.w(TAG, "could not unregister network callback", t)
        } finally {
            registered = false
            currentNetwork = null
        }
    }

    private companion object {
        const val TAG = "NetworkMonitor"
    }
}
