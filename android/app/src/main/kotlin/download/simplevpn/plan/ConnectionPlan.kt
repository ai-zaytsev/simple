package download.simplevpn.plan

import download.simplevpn.config.ConnectionProfile
import download.simplevpn.config.RoutingPolicy
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

    // Where traffic goes. Part of the plan rather than of the build, which is
    // the whole point of the stage: a route that turns out wrong is corrected
    // by an operator, not by a release.
    val routing: RoutingPolicy,
) {
    /** Everything to try, in the order the server chose. */
    val endpoints: List<ConnectionProfile> get() = listOf(primary) + reserves

    /**
     * What this plan actually says, ignoring which copy of it this is.
     *
     * The sequence number is not it. The server issues a new number on every
     * request, so two plans a second apart are different numbers and identical
     * instructions - and treating "new number" as "new plan" made the rollback
     * inert: every retry fetched a fresh number, the failure count reset, and
     * the same broken plan was tried for ever. Found by walking through the
     * test before running it.
     *
     * Endpoints and routing, because those are what can be wrong. A plan whose
     * timeouts changed is not a plan that deserves a fresh chance after two
     * failures; a plan naming a different node is.
     */
    val fingerprint: String
        get() = buildString {
            endpoints.forEach { append(it.host).append(':').append(it.port).append(' ') }
            append('|')
            append(routing.directApps.joinToString(","))
            append('|').append(routing.directDomains.joinToString(","))
            append('|').append(routing.directIPs.joinToString(","))
            append('|').append(routing.proxyDomains.joinToString(","))
            append('|').append(routing.proxyIPs.joinToString(","))
            append('|').append(routing.russiaDirect)
        }

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
                    routing = RoutingPolicy.parse(payload.optJSONObject("routing")),
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
