package download.simplevpn.core

import android.util.Log
import tunbridge.Tunbridge

/**
 * Moves packets between the TUN interface and the engine's local proxy.
 *
 * The engine does not read the network interface: it exposes a SOCKS proxy on
 * loopback and nothing more. This is the piece that makes device traffic
 * actually reach it, and without it the application connects to the node while
 * every packet still goes around the tunnel.
 *
 * Ownership of the file descriptor moves here. The caller detaches it from its
 * Java owner first, because a descriptor closed by both sides is closed twice.
 */
class TunBridge {

    val isRunning: Boolean
        get() = try {
            Tunbridge.isRunning()
        } catch (t: Throwable) {
            false
        }

    /**
     * @param tunFd detached descriptor of the established interface
     * @param mtu must match the value the interface was built with
     */
    fun start(tunFd: Int, mtu: Int, socksPort: Int): Result {
        return try {
            Tunbridge.start(tunFd.toLong(), mtu.toLong(), "socks5://127.0.0.1:$socksPort")
            Result.Started
        } catch (t: UnsatisfiedLinkError) {
            Log.e(TAG, "bridge library is missing", t)
            Result.Unavailable("Packet bridge is not bundled in this build")
        } catch (t: Throwable) {
            Log.e(TAG, "bridge failed to start", t)
            Result.Failed(t.message ?: "Packet bridge failed to start")
        }
    }

    fun stop() {
        try {
            Tunbridge.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "bridge stop failed", t)
        }
    }

    sealed interface Result {
        data object Started : Result
        data class Unavailable(val reason: String) : Result
        data class Failed(val reason: String) : Result
    }

    private companion object {
        const val TAG = "TunBridge"
    }
}
