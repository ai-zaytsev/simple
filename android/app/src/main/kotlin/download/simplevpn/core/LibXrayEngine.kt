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

    override fun stop() {
        try {
            Vpncore.stopEngine()
        } catch (t: Throwable) {
            Log.w(TAG, "engine stop failed", t)
        } finally {
            running = false
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
    }
}
