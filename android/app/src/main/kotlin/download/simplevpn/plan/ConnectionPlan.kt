package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.TransportParams
import org.json.JSONObject

/**
 * What the server decided, as the client understands it.
 *
 * Only the fields the client acts on are modelled. A plan carrying something
 * this version does not know about is still a valid plan: unknown fields are
 * ignored rather than treated as corruption, which is what lets the server
 * introduce a field before every installation understands it.
 *
 * The opposite is not true. A missing required field makes the plan invalid as
 * a whole, and no part of it is applied. Partial application would mean a
 * client running half of one plan and half of another.
 */
data class ConnectionPlan(
    val seq: Long,
    val expiresAt: Long,
    val primary: ConnectionProfile,
    val reserves: List<ConnectionProfile>,
    val refreshAfterSeconds: Int,

    // Numbers the server chose so the client does not invent them. A client
    // that picks its own timeouts and its own idea of when a node is dead is a
    // client making policy, and policy has to be changeable without an update.
    val connectTimeoutMs: Int,
    val failoverAfterFailures: Int,
    val probeIntervalSeconds: Int,
) {
    /** Everything to try, in the order the server chose. */
    val endpoints: List<ConnectionProfile> get() = listOf(primary) + reserves

    companion object {

        fun parse(payload: JSONObject): ConnectionPlan? {
            return try {
                val version = payload.getInt("v")
                if (version != SUPPORTED_VERSION) return null

                val primary = node(payload.getJSONObject("primary")) ?: return null

                // A node whose transport this build cannot speak is skipped,
                // not fatal. During a transport migration the server issues
                // mixed plans on purpose, and refusing the whole plan would
                // strand exactly the clients being migrated.
                val reserves = mutableListOf<ConnectionProfile>()
                val array = payload.optJSONArray("reserves")
                if (array != null) {
                    for (i in 0 until array.length()) {
                        node(array.optJSONObject(i) ?: continue)?.let(reserves::add)
                    }
                }

                val policy = payload.optJSONObject("policy")

                ConnectionPlan(
                    seq = payload.getLong("seq"),
                    expiresAt = Instants.parse(payload.getString("expires_at")) ?: return null,
                    primary = primary,
                    reserves = reserves,
                    refreshAfterSeconds = policy?.optInt("plan_refresh_after_s", DEFAULT_REFRESH)
                        ?: DEFAULT_REFRESH,
                    connectTimeoutMs = policy?.optInt("connect_timeout_ms", DEFAULT_CONNECT_TIMEOUT)
                        ?: DEFAULT_CONNECT_TIMEOUT,
                    failoverAfterFailures = policy?.optInt("failover_after_failures", DEFAULT_FAILOVER)
                        ?: DEFAULT_FAILOVER,
                    probeIntervalSeconds = policy?.optInt("probe_interval_s", DEFAULT_PROBE_INTERVAL)
                        ?: DEFAULT_PROBE_INTERVAL,
                )
            } catch (t: Throwable) {
                null
            }
        }

        private fun node(json: JSONObject): ConnectionProfile? {
            return try {
                val transport = json.getJSONObject("transport")
                val kind = transport.getString("kind")
                val params = transport.getJSONObject("params")

                val shaped = when (kind) {
                    TransportParams.VlessWsTls.KIND -> TransportParams.VlessWsTls(
                        credentialUuid = params.getString("credential_uuid"),
                        path = params.getString("path"),
                        serverName = params.getString("server_name"),
                        hostHeader = params.optString("host_header", params.getString("server_name")),
                        fingerprint = params.optString("fingerprint", "chrome"),
                    )

                    TransportParams.VlessReality.KIND -> TransportParams.VlessReality(
                        credentialUuid = params.getString("credential_uuid"),
                        flow = params.optString("flow", "xtls-rprx-vision"),
                        serverName = params.getString("server_name"),
                        publicKey = params.getString("public_key"),
                        shortId = params.optString("short_id", ""),
                        fingerprint = params.optString("fingerprint", "chrome"),
                    )

                    // Not an error: a transport this build does not know about.
                    else -> return null
                }

                ConnectionProfile(
                    alias = json.getString("alias"),
                    host = json.getString("host"),
                    port = json.getInt("port"),
                    transport = shaped,
                )
            } catch (t: Throwable) {
                null
            }
        }

        private const val SUPPORTED_VERSION = 1
        private const val DEFAULT_REFRESH = 43_200
        private const val DEFAULT_CONNECT_TIMEOUT = 8_000
        private const val DEFAULT_FAILOVER = 2
        private const val DEFAULT_PROBE_INTERVAL = 60
    }
}
