package download.simplevpn.core

import android.util.Log
import java.net.InetSocketAddress
import java.net.Socket
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

/**
 * Opens a plain TLS connection to the node, from the application, with the
 * tunnel up and without involving the engine.
 *
 * This answers the one question the other checks leave open. A browser can
 * reach the node with the VPN switched off; that proves the network allows it,
 * not that the application's own traffic escapes the tunnel it is running.
 *
 * The application excludes itself from its own tunnel, because otherwise the
 * connection the engine makes towards the node is routed into the tunnel that
 * connection is meant to establish - it arrives back at the engine and waits
 * for itself. From outside, that loop looks exactly like an unreachable node.
 *
 *   handshake succeeds -> the application's traffic escapes and the node
 *                         answers, so the fault is in how the engine uses it
 *   handshake times out -> the traffic does not escape, and the loop is real
 *
 * Certificate validation is left on. A node that answers with the wrong
 * certificate is a different failure and should not be reported as reachable.
 *
 * Temporary, and goes when the slice does.
 */
object NodeReachTest {

    sealed interface Result {
        data class Reached(val certificateName: String) : Result
        data class Failed(val reason: String) : Result
    }

    fun run(host: String, port: Int, serverName: String): Result {
        return try {
            Socket().use { plain ->
                plain.connect(InetSocketAddress(host, port), TIMEOUT_MS)
                plain.soTimeout = TIMEOUT_MS

                val factory = SSLSocketFactory.getDefault() as SSLSocketFactory
                // serverName rather than host: the certificate carries the
                // name, while the address only selects the machine.
                val tls = factory.createSocket(plain, serverName, port, false) as SSLSocket
                tls.soTimeout = TIMEOUT_MS
                tls.startHandshake()

                val subject = tls.session.peerCertificates
                    .firstOrNull()
                    ?.let { certificate ->
                        runCatching {
                            javax.security.auth.x500.X500Principal(
                                (certificate as java.security.cert.X509Certificate)
                                    .subjectX500Principal.name,
                            ).name
                        }.getOrNull()
                    }

                tls.close()
                Result.Reached(subject?.take(MAX_NAME_LENGTH) ?: "no name")
            }
        } catch (t: Throwable) {
            Log.w(TAG, "node not reachable from inside the tunnel", t)
            Result.Failed(t.message ?: t.javaClass.simpleName)
        }
    }

    private const val TAG = "NodeReachTest"
    private const val TIMEOUT_MS = 10_000
    private const val MAX_NAME_LENGTH = 40
}
