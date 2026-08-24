package download.simplevpn.net

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.util.Log

/**
 * Reports when the network the tunnel actually runs over goes away.
 *
 * The TUN interface survives a change of underlying network, but sockets the
 * engine opened over the old one do not: they stay open and never deliver
 * anything. Without reacting, the tunnel looks established and passes no
 * traffic, which is the worst of both worlds.
 *
 * What this deliberately does NOT report is another network merely appearing.
 * The callback fires for every network matching the request, and a phone with
 * Wi-Fi connected usually has mobile data up as well. Treating the second
 * arrival as a change restarts a tunnel that is working, seconds after it came
 * up, and a restart that races the stop it follows is refused by the engine as
 * "already running" - which used to tear the whole tunnel down. A network
 * appearing beside the one in use is not a change of the one in use.
 *
 * Only networks that are not themselves a VPN are considered, so the tunnel
 * does not react to its own arrival.
 */
class NetworkMonitor(
    context: Context,
    private val onUnderlyingNetworkChanged: (Network) -> Unit,
    private val onNetworkLost: () -> Unit,
) {

    private val connectivityManager =
        context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

    /**
     * Every usable network, in arrival order. Kept because losing the one in
     * use has to be answered with another one immediately, and by then the
     * system has already announced the alternatives and will not repeat itself.
     */
    private val available = LinkedHashSet<Network>()

    private var currentNetwork: Network? = null

    /** False until the first network is adopted, so starting is not a change. */
    private var everAdopted = false
    private var registered = false

    private val callback = object : ConnectivityManager.NetworkCallback() {

        override fun onAvailable(network: Network) {
            synchronized(available) {
                available.add(network)
                if (currentNetwork != null) {
                    Log.i(TAG, "another network is available, keeping the one in use")
                    return
                }
                adopt(network)
            }
        }

        override fun onLost(network: Network) {
            synchronized(available) {
                available.remove(network)
                if (currentNetwork != network) return

                currentNetwork = null
                val replacement = available.lastOrNull()
                if (replacement != null) {
                    Log.i(TAG, "network in use was lost, another is available")
                    adopt(replacement)
                } else {
                    Log.i(TAG, "no usable network left")
                    onNetworkLost()
                }
            }
        }
    }

    private fun adopt(network: Network) {
        currentNetwork = network
        if (everAdopted) {
            onUnderlyingNetworkChanged(network)
        }
        everAdopted = true
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
            synchronized(available) {
                available.clear()
                currentNetwork = null
                everAdopted = false
            }
        }
    }

    private companion object {
        const val TAG = "NetworkMonitor"
    }
}
