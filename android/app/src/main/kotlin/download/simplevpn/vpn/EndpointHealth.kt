package download.simplevpn.vpn

import android.util.Log
import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.TransportParams
import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetSocketAddress
import java.net.Socket
import java.security.SecureRandom
import java.util.Base64
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
 * The request sent is the one the engine itself sends. A health check with a
 * shape of its own would be a second thing to fingerprint, visible to anyone
 * watching the connection and present on no other client.
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
            plain = Socket()
            protect(plain)
            plain.connect(InetSocketAddress(profile.host, profile.port), timeoutMs)
            plain.soTimeout = timeoutMs

            val tls = SSLSocketFactory.getDefault()
                .createSocket(plain, transport.serverName, profile.port, false) as SSLSocket

            // The name in the handshake, exactly as the engine sends it: the
            // node is dialled by address and recognises itself by this.
            tls.sslParameters = tls.sslParameters.apply {
                serverNames = listOf(javax.net.ssl.SNIHostName(transport.serverName))
                // Only what the transport speaks. Offering h2 here once made a
                // perfect handshake end in a hang, because the upgrade that
                // follows exists only over HTTP/1.1.
                applicationProtocols = arrayOf("http/1.1")
            }
            tls.startHandshake()

            val key = ByteArray(16).also { SecureRandom().nextBytes(it) }
            val request = buildString {
                append("GET ").append(transport.path).append(" HTTP/1.1\r\n")
                append("Host: ").append(transport.hostHeader).append("\r\n")
                append("Upgrade: websocket\r\n")
                append("Connection: Upgrade\r\n")
                append("Sec-WebSocket-Key: ").append(Base64.getEncoder().encodeToString(key)).append("\r\n")
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
