package download.simplevpn.vpn

import android.util.Log
import download.simplevpn.config.XrayConfigBuilder
import download.simplevpn.plan.ControlPlane
import java.net.HttpURLConnection
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.URL

/**
 * Whether the tunnel actually carries traffic, asked by carrying some.
 *
 * "Connected" has never meant "working". The engine starts, the interface is
 * up, the notification says connected - and nothing loads, because the node
 * refuses the credential, or a routing rule sends everything nowhere, or the
 * plan names an endpoint that has been withdrawn. Every one of those has
 * happened in this project, and each looked like success from the inside.
 *
 * The proof goes through the engine's own proxy rather than through the
 * interface, and that is not a shortcut: the application excludes itself from
 * its own tunnel, so a request it makes normally would never enter the tunnel
 * at all and would prove nothing. Asking the engine directly exercises the
 * whole path that matters - node reachable, credential accepted, routing
 * sensible, name resolved, bytes back.
 */
object TunnelProof {

    /**
     * @return true when a request completed through the engine
     */
    fun carriesTraffic(timeoutMs: Int): Boolean {
        var connection: HttpURLConnection? = null
        return try {
            val proxy = Proxy(
                Proxy.Type.SOCKS,
                InetSocketAddress("127.0.0.1", XrayConfigBuilder.SOCKS_PORT),
            )

            // The Control Plane, because it is ours, small, always up, and a
            // foreign name - so routing sends it through the tunnel rather than
            // straight out, which is the half being tested.
            val url = URL(ControlPlane.BASE_URL + "/v1/config")
            connection = (url.openConnection(proxy) as HttpURLConnection).apply {
                requestMethod = "GET"
                connectTimeout = timeoutMs
                readTimeout = timeoutMs
                setRequestProperty("accept", "application/json")
            }

            val code = connection.responseCode
            // Any answer at all is proof: the bytes crossed the tunnel and came
            // back. What the answer says is a different question, asked
            // elsewhere.
            val carried = code in 200..499
            if (!carried) Log.w(TAG, "tunnel answered $code")
            carried
        } catch (t: Throwable) {
            Log.w(TAG, "nothing came back through the tunnel: ${t.message}")
            false
        } finally {
            connection?.disconnect()
        }
    }

    private const val TAG = "TunnelProof"
}
