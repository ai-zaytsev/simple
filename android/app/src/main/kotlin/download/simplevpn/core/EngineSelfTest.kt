package download.simplevpn.core

import android.util.Log
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.Socket

/**
 * Asks the engine to fetch one page, without going through the tunnel.
 *
 * This is the cut that the packet counters cannot make. They show what reaches
 * the bridge; they say nothing about whether the leg beyond it - engine to node
 * - works on this particular device and network. Going straight to the engine's
 * local proxy tests exactly that leg and nothing else:
 *
 *   succeeds -> the engine reaches the node, so the fault is between the
 *               interface and the engine
 *   fails    -> the engine cannot reach the node from this device, and the
 *               bridge was never the problem
 *
 * The address is deliberately left for the proxy to resolve, so the check also
 * covers name resolution through the tunnel, which is what a browser needs
 * first and what no local check would exercise.
 *
 * The application is excluded from its own tunnel, so this call reaches
 * loopback directly and does not disturb what is being measured.
 *
 * Temporary, and goes when the slice does.
 */
object EngineSelfTest {

    sealed interface Result {
        data class Reached(val exitAddress: String) : Result
        data class Failed(val reason: String) : Result
    }

    fun run(socksPort: Int): Result {
        return try {
            val proxy = Proxy(Proxy.Type.SOCKS, InetSocketAddress(LOOPBACK, socksPort))
            Socket(proxy).use { socket ->
                socket.soTimeout = TIMEOUT_MS
                // Unresolved on purpose: the proxy is asked to resolve it.
                socket.connect(InetSocketAddress.createUnresolved(HOST, PORT), TIMEOUT_MS)

                socket.getOutputStream().write(REQUEST.toByteArray())
                socket.getOutputStream().flush()

                val response = socket.getInputStream().bufferedReader().readText()
                val body = response.substringAfter(HEADER_END, "").trim()

                if (body.isEmpty()) {
                    Result.Failed("empty answer")
                } else {
                    Result.Reached(body.take(MAX_ADDRESS_LENGTH))
                }
            }
        } catch (t: Throwable) {
            Log.w(TAG, "engine could not fetch through its own proxy", t)
            Result.Failed(t.message ?: t.javaClass.simpleName)
        }
    }

    private const val TAG = "EngineSelfTest"
    private const val LOOPBACK = "127.0.0.1"
    private const val HOST = "api.ipify.org"
    private const val PORT = 80
    private const val TIMEOUT_MS = 15_000
    private const val HEADER_END = "\r\n\r\n"
    private const val MAX_ADDRESS_LENGTH = 45
    private const val REQUEST =
        "GET / HTTP/1.1\r\nHost: $HOST\r\nConnection: close\r\n\r\n"
}
