package download.simplevpn.core

import android.net.VpnService
import android.util.Log
import libXray.DialerController
import libXray.LibXray
import org.json.JSONObject

/**
 * The real transport engine, backed by the libXray AAR built from XTLS source
 * (ADR-020).
 *
 * The engine is controlled through a single native entry point that takes a
 * JSON request and returns a JSON response. That means the contract is a
 * message format, not a set of method signatures: a wrong method name compiles
 * fine and fails at runtime. The method names live in [Method] and the response
 * is always checked, so a failure surfaces as a reported error rather than as a
 * tunnel that silently carries nothing.
 *
 * What this class does NOT do: move packets. libXray runs Xray and exposes a
 * local SOCKS inbound; nothing in it reads the TUN interface. Feeding device
 * traffic into that inbound is a separate component.
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
    private val dialerController = DialerController { fd ->
        val ok = vpnService.protect(fd.toInt())
        if (!ok) {
            Log.w(TAG, "socket could not be protected, traffic may loop")
        }
        ok
    }

    override fun start(configJson: String, tunFd: Int): EngineStartResult {
        return try {
            LibXray.registerDialerController(dialerController)

            val payload = JSONObject().put("xrayJson", configJson)
            val response = invoke(Method.RUN, payload)

            if (!response.success) {
                EngineStartResult.Failed(response.error ?: "Engine refused the configuration")
            } else {
                running = true
                EngineStartResult.Started
            }
        } catch (t: UnsatisfiedLinkError) {
            // The AAR is absent from this build. Distinct from a runtime
            // failure: it is a build-time fact and the user-facing message
            // differs accordingly.
            Log.e(TAG, "engine library is missing", t)
            EngineStartResult.Unavailable("Transport engine is not bundled in this build")
        } catch (t: Throwable) {
            Log.e(TAG, "engine failed to start", t)
            EngineStartResult.Failed(t.message ?: "Engine failed to start", t)
        }
    }

    override fun stop() {
        try {
            invoke(Method.STOP, null)
        } catch (t: Throwable) {
            Log.w(TAG, "engine stop failed", t)
        } finally {
            running = false
        }
    }

    /** Asks the engine itself rather than trusting the local flag. */
    fun queryRunning(): Boolean = try {
        val response = invoke(Method.STATE, null)
        response.success && response.data?.optBoolean("running", false) == true
    } catch (t: Throwable) {
        Log.w(TAG, "state query failed", t)
        false
    }

    private fun invoke(method: Method, payload: JSONObject?): Response {
        val request = JSONObject().apply {
            put("apiVersion", API_VERSION)
            put("method", method.wire)
            if (payload != null) {
                put("payload", payload)
            }
        }

        val raw = LibXray.invoke(request.toString())
        val json = JSONObject(raw)

        return Response(
            success = json.optBoolean("success", false),
            data = json.optJSONObject("data"),
            error = json.optString("error").takeIf { it.isNotEmpty() },
        )
    }

    private data class Response(
        val success: Boolean,
        val data: JSONObject?,
        val error: String?,
    )

    /**
     * The engine's method vocabulary. Kept as a closed set because a typo in a
     * method name is not a compile error on the other side of the boundary.
     */
    private enum class Method(val wire: String) {
        RUN("runXray"),
        STOP("stopXray"),
        STATE("getXrayState"),
    }

    private companion object {
        const val TAG = "LibXrayEngine"

        /**
         * The engine rejects a request whose version it does not recognise, so
         * this must match the AAR that is bundled. Pinned together with the
         * engine build; see docs/integrations/libxray.md.
         */
        const val API_VERSION = 2
    }
}
