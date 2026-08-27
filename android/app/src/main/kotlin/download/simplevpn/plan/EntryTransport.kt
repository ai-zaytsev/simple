package download.simplevpn.plan

import android.util.Log
import java.net.HttpURLConnection
import java.net.Socket
import java.net.URL
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SNIHostName
import javax.net.ssl.SSLSocket
import javax.net.ssl.SSLSocketFactory

/**
 * Opens a connection to one entry, including the kind that uses no resolver.
 *
 * An address entry exists to survive a blocked or poisoned resolver, and that
 * only works if nothing along the way asks a resolver anything. Putting the
 * name in the address bar and hoping a custom socket factory avoids the lookup
 * does not work here: the HTTP stack resolves the name first and connects
 * afterwards, so the lookup happens whatever the factory does.
 *
 * So the address goes in the address bar, and the name is supplied where it is
 * actually needed - in the handshake, in the certificate check, and in the Host
 * header. Nothing is weakened: the certificate is still verified, and still
 * against the name we expect rather than against the address we dialled.
 */
object EntryTransport {

    /**
     * @param throughProxy when set, the request goes out through the engine.
     *   That is the tunnel channel: with every public way in blocked, a device
     *   that still has a working plan raises the tunnel on it and asks from the
     *   inside, where the block does not reach. It is also why an old plan is
     *   worth keeping - it is the only key left when every door is shut.
     */
    fun open(
        entry: Entry,
        path: String,
        timeoutMs: Int,
        throughProxy: java.net.Proxy? = null,
    ): HttpURLConnection {
        val url = URL(entry.baseUrl + path)
        val connection = (
            if (throughProxy != null) url.openConnection(throughProxy) else url.openConnection()
        ) as HttpURLConnection
        connection.connectTimeout = timeoutMs
        connection.readTimeout = timeoutMs

        if (entry.kind == Entry.Kind.ADDRESS && connection is HttpsURLConnection) {
            val expected = entry.expectedName
            connection.sslSocketFactory = sniFactory(expected)

            // The address will not match the certificate, and it is not meant
            // to. What must match is the name, and this checks exactly that
            // using the platform's own verifier rather than accepting anything.
            connection.hostnameVerifier = HostnameVerifier { _, session ->
                val default = HttpsURLConnection.getDefaultHostnameVerifier()
                default.verify(expected, session)
            }

            // Without this the server is asked for the address, and any host
            // that serves more than one name would answer with the wrong one.
            connection.setRequestProperty("host", expected)
        }

        return connection
    }

    /**
     * A factory that puts the expected name in the handshake.
     *
     * Needed because the socket is opened to an address: without this the
     * handshake carries no name, and a server with several certificates has no
     * way to choose the right one.
     */
    private fun sniFactory(name: String): SSLSocketFactory {
        val delegate = SSLSocketFactory.getDefault() as SSLSocketFactory
        return object : SSLSocketFactory() {
            override fun getDefaultCipherSuites(): Array<String> = delegate.defaultCipherSuites
            override fun getSupportedCipherSuites(): Array<String> = delegate.supportedCipherSuites

            override fun createSocket(s: Socket?, host: String?, port: Int, autoClose: Boolean): Socket =
                withName(delegate.createSocket(s, host, port, autoClose))

            override fun createSocket(host: String?, port: Int): Socket =
                withName(delegate.createSocket(host, port))

            override fun createSocket(host: String?, port: Int, localHost: java.net.InetAddress?, localPort: Int): Socket =
                withName(delegate.createSocket(host, port, localHost, localPort))

            override fun createSocket(host: java.net.InetAddress?, port: Int): Socket =
                withName(delegate.createSocket(host, port))

            override fun createSocket(address: java.net.InetAddress?, port: Int, localAddress: java.net.InetAddress?, localPort: Int): Socket =
                withName(delegate.createSocket(address, port, localAddress, localPort))

            private fun withName(socket: Socket): Socket {
                if (socket is SSLSocket) {
                    try {
                        socket.sslParameters = socket.sslParameters.apply {
                            serverNames = listOf(SNIHostName(name))
                        }
                    } catch (t: Throwable) {
                        Log.w(TAG, "could not set the name in the handshake", t)
                    }
                }
                return socket
            }
        }
    }

    private const val TAG = "EntryTransport"
}
