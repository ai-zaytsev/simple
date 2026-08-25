package download.simplevpn.plan

import android.content.Context
import android.util.Log
import org.json.JSONObject

/**
 * The switch that can stop this installation, and the only path by which it
 * arrives.
 *
 * The stored document outlives failure to reach the server, and that asymmetry
 * is the point. A plan that cannot be refreshed keeps working, because the
 * nodes it names are probably still there. A configuration that cannot be
 * refreshed also keeps working - which means a kill switch already received
 * stays on even if the server becomes unreachable a second later.
 *
 * What cannot be defended against here is the switch never arriving: somebody
 * who can block this service can stop it being thrown. The remedy for that is
 * not in the client, and pretending otherwise - refusing to run when the
 * server is unreachable - would hand exactly that adversary the ability to
 * disconnect everybody by blocking one address.
 */
class ConfigSource(private val context: Context) {

    private val prefs = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)
    private val client = ControlPlaneClient(context)

    /** Must not be called on the main thread: it may open a connection. */
    fun current(now: Long = System.currentTimeMillis()): ServiceConfig? {
        val stored = stored()

        val fresh = stored != null && now < prefs.getLong(KEY_FETCHED_AT, 0L) +
            stored.refreshAfterSeconds * 1000L
        if (fresh) return stored

        val fetched = fetch(now)
        if (fetched != null) return fetched

        // Kept rather than discarded, and kept whatever its age. See above:
        // this is the difference between a switch that survives an outage and
        // one an adversary can clear by cutting a wire.
        return stored
    }

    private fun fetch(now: Long): ServiceConfig? {
        val response = when (val answer = client.requestConfig()) {
            is ControlPlaneClient.Result.Received -> answer.envelopeJson
            is ControlPlaneClient.Result.Failed -> {
                Log.i(TAG, "could not refresh configuration: ${answer.reason}")
                return null
            }
        }

        // Signature first, always. A document that decides whether this
        // installation may run is the last place to trust an unverified body.
        val payload = when (val opened = SignedDocument.open(response)) {
            is SignedDocument.Result.Trusted -> opened.payload
            is SignedDocument.Result.Rejected -> {
                Log.w(TAG, "configuration rejected: ${opened.reason}")
                return null
            }
        }

        val candidate = ServiceConfig.parse(payload) ?: run {
            Log.w(TAG, "configuration is not usable by this build")
            return null
        }

        val current = stored()
        if (!ServiceConfig.supersedes(candidate, current)) {
            Log.w(TAG, "configuration is not newer than the one in use")
            return current
        }

        prefs.edit()
            .putString(KEY_DOCUMENT, payload.toString())
            .putLong(KEY_SEQ, candidate.seq)
            .putLong(KEY_FETCHED_AT, now)
            .apply()
        return candidate
    }

    private fun stored(): ServiceConfig? {
        val raw = prefs.getString(KEY_DOCUMENT, null) ?: return null
        return try {
            ServiceConfig.parse(JSONObject(raw))
        } catch (t: Throwable) {
            Log.w(TAG, "stored configuration is unreadable", t)
            null
        }
    }

    private companion object {
        const val TAG = "ConfigSource"
        const val NAME = "service_config"
        const val KEY_DOCUMENT = "payload"
        const val KEY_SEQ = "seq"
        const val KEY_FETCHED_AT = "fetched_at"
    }
}
