package download.simplevpn.core

import android.net.VpnService
import android.util.Log
import vpncore.Protector
import vpncore.Vpncore

/**
 * The real transport engine, backed by Xray through libXray, built from XTLS
 * source at a pinned commit (ADR-020).
 *
 * What this class does NOT do: move packets. The engine runs Xray and exposes
 * a local SOCKS inbound; nothing in it reads the TUN interface. Feeding device
 * traffic into that inbound is [TunBridge], and both live in the same native
 * library because two gomobile bindings cannot coexist in one APK.
 */
class LibXrayEngine(private val vpnService: VpnService) : XrayEngine {

    @Volatile
    private var running = false

    override val isRunning: Boolean
        get() = running

    /**
     * Hands every socket the engine opens to the system for protection.
     *
     * A protected socket bypasses the TUN interface. Without this the engine's
     * own connection to the node would be routed back into the tunnel it is
     * trying to establish, which is the loop that makes a VPN appear to hang.
     * The application also excludes itself from the tunnel, so this is the
     * second of two independent guards against the same failure.
     */
    private val protector = Protector { fd ->
        val ok = vpnService.protect(fd.toInt())
        if (!ok) {
            Log.w(TAG, "socket could not be protected, traffic may loop")
        }
        ok
    }

    override fun start(configJson: String, tunFd: Int): EngineStartResult {
        return try {
            val assets = GeoAssets.install(vpnService)
                ?: return EngineStartResult.Unavailable(
                    "Routing database is missing from this build",
                )

            // Order matters: both must be in place before the engine reads the
            // configuration, because the configuration references the database
            // and the first socket may be opened immediately.
            Vpncore.setAssetDir(assets.absolutePath)
            Vpncore.setProtector(protector)

            // Never on top of one that is still alive. Two engines share the
            // local proxy port, and whichever holds it decides where traffic
            // goes - which once sent everything to a node that had just been
            // abandoned, through a tunnel that reported success.
            if (queryRunning()) {
                Log.w(TAG, "an engine is already running, stopping it first")
                stop()
            }

            // A refused configuration arrives here as a thrown exception, which
            // is how the binding reports a Go error.
            Vpncore.startEngine(configJson)

            running = true
            EngineStartResult.Started
        } catch (t: UnsatisfiedLinkError) {
            // The native library is absent from this build. Distinct from a
            // runtime failure: it is a build-time fact and the user-facing
            // message differs accordingly.
            Log.e(TAG, "engine library is missing", t)
            EngineStartResult.Unavailable("Transport engine is not bundled in this build")
        } catch (t: Throwable) {
            Log.e(TAG, "engine failed to start", t)
            EngineStartResult.Failed(t.message ?: "Engine failed to start", t)
        }
    }

    /**
     * Stops the engine and waits until it has actually stopped.
     *
     * Asking it to stop is not the same as it having stopped, and the
     * difference showed up as one of the strangest failures in this project: a
     * plan was rolled back, a new engine was started on a working node, and
     * traffic still went to the address of the plan that had been abandoned.
     * Two engines were alive at once. The old one still held the local proxy
     * port, so everything handed to that port went out through the endpoint
     * nobody wanted any more - a rebuild that looked complete and changed
     * nothing.
     *
     * The engine is asked whether it is running rather than told. A local flag
     * says what was requested; this says what is true.
     */
    override fun stop() {
        try {
            Vpncore.stopEngine()
        } catch (t: Throwable) {
            Log.w(TAG, "engine stop failed", t)
        } finally {
            running = false
        }

        val deadline = System.currentTimeMillis() + STOP_TIMEOUT_MS
        while (queryRunning() && System.currentTimeMillis() < deadline) {
            try {
                Thread.sleep(STOP_POLL_MS)
            } catch (interrupted: InterruptedException) {
                Thread.currentThread().interrupt()
                return
            }
        }

        if (queryRunning()) {
            // Worth saying out loud rather than continuing quietly. Starting a
            // second engine now is what produced the failure above, and a log
            // that says so turns an inexplicable result into a named one.
            Log.w(TAG, "engine still reports running after ${STOP_TIMEOUT_MS}ms")
        }
    }

    /** Asks the engine itself rather than trusting the local flag. */
    fun queryRunning(): Boolean = try {
        Vpncore.engineRunning()
    } catch (t: Throwable) {
        Log.w(TAG, "state query failed", t)
        false
    }

    private companion object {
        const val TAG = "LibXrayEngine"

        // Long enough for an orderly shutdown, short enough that a stuck
        // engine does not hold a reconnection for ever.
        const val STOP_TIMEOUT_MS = 3_000L
        const val STOP_POLL_MS = 25L
    }
}
