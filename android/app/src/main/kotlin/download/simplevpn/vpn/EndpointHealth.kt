package download.simplevpn.vpn

import android.util.Base64
import android.util.Log
import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.TransportParams
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetSocketAddress
import java.net.Socket
import java.security.SecureRandom
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

/**
 * Whether an endpoint can actually carry a tunnel.
 *
 * The first version of this asked whether the port accepted a connection, and
 * that was nearly useless. Port 443 belongs to Nginx, which serves an ordinary
 * website and answers whether or not the tunnel behind it is alive. A node
 * whose Xray had died would have passed every check while carrying nothing,
 * and failover - the entire point - would never have fired.
 *
 * So the question asked is the one that matters: does the path that carries
 * the tunnel complete a WebSocket upgrade. Nginx answers 101 only when it
 * reached the engine behind it; with the engine gone it answers 502, and with
 * the machine gone nothing answers at all. All three are now distinguishable
 * from here.
 *
 * The HTTP request sent is the one the engine itself sends. A health check
 * with a shape of its own would be a second thing to fingerprint, visible to
 * anyone watching the connection and present on no other client.
 *
 * The TLS handshake underneath is the platform's, which is not the same as the
 * engine's. That is a real if small difference and it is the price of asking
 * at all; the alternative is no failover.
 */
class EndpointHealth(private val protect: (Socket) -> Boolean) {

    fun check(profile: ConnectionProfile, timeoutMs: Int): Boolean {
        return when (val transport = profile.transport) {
            is TransportParams.VlessWsTls -> upgrades(profile, transport, timeoutMs)

            // REALITY has no cheap equivalent: there is no plaintext exchange
            // to look at before the credential is presented. A connection that
            // is accepted is all this can honestly report.
            else -> accepts(profile, timeoutMs)
        }
    }

    private fun accepts(profile: ConnectionProfile, timeoutMs: Int): Boolean = try {
        Socket().use { socket ->
            protect(socket)
            socket.connect(InetSocketAddress(profile.host, profile.port), timeoutMs)
            true
        }
    } catch (t: Throwable) {
        Log.i(TAG, "endpoint does not accept connections: ${t.message}")
        false
    }

    private fun upgrades(
        profile: ConnectionProfile,
        transport: TransportParams.VlessWsTls,
        timeoutMs: Int,
    ): Boolean {
        var plain: Socket? = null
        return try {
            val socket = Socket()
            plain = socket
            protect(socket)
            socket.connect(InetSocketAddress(profile.host, profile.port), timeoutMs)
            socket.soTimeout = timeoutMs

            // Cast because getDefault() is declared to return a SocketFactory,
            // and only the SSL one can wrap a socket that is already connected.
            val factory = SSLSocketFactory.getDefault() as SSLSocketFactory
            val tls = factory
                .createSocket(socket, transport.serverName, profile.port, false) as SSLSocket
            tls.soTimeout = timeoutMs

            // The name in the handshake, exactly as the engine sends it: the
            // node is dialled by address and recognises itself by this.
            //
            // No ALPN is offered, deliberately. Nginx serves HTTP/2 only when a
            // client asks for it by name, and the upgrade this check depends on
            // exists only over HTTP/1.1 - a lesson already paid for once, when
            // offering h2 turned a perfect handshake into a hang. Saying
            // nothing gets HTTP/1.1 and works on every Android this build
            // supports; naming the protocol needs API 29.
            tls.sslParameters = tls.sslParameters.apply {
                serverNames = listOf(SNIHostName(transport.serverName))
            }
            tls.startHandshake()

            val key = ByteArray(16).also { SecureRandom().nextBytes(it) }
            val request = buildString {
                append("GET ").append(transport.path).append(" HTTP/1.1\r\n")
                append("Host: ").append(transport.hostHeader).append("\r\n")
                append("Upgrade: websocket\r\n")
                append("Connection: Upgrade\r\n")
                // Android's encoder, not java.util.Base64: the latter arrived
                // in API 26 and this build runs from 24.
                append("Sec-WebSocket-Key: ").append(Base64.encodeToString(key, Base64.NO_WRAP)).append("\r\n")
                append("Sec-WebSocket-Version: 13\r\n")
                append("\r\n")
            }
            tls.outputStream.write(request.toByteArray())
            tls.outputStream.flush()

            val status = BufferedReader(InputStreamReader(tls.inputStream)).readLine()
            tls.close()

            val upgraded = status != null && status.contains("101")
            if (!upgraded) {
                // Worth the words: this is a live machine serving a live site
                // whose tunnel is not there. It is the failure that used to be
                // invisible.
                Log.i(TAG, "endpoint answered but did not upgrade: $status")
            }
            upgraded
        } catch (t: Throwable) {
            Log.i(TAG, "endpoint did not complete an upgrade: ${t.message}")
            false
        } finally {
            try {
                plain?.close()
            } catch (_: Throwable) {
                // Already closed by the TLS socket wrapping it.
            }
        }
    }

    private companion object {
        const val TAG = "EndpointHealth"
    }
}
