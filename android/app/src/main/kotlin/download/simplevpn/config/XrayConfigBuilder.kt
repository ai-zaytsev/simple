package download.simplevpn.config

import org.json.JSONArray
import org.json.JSONObject

/**
 * Builds the engine configuration from a profile and a routing policy.
 *
 * Two properties of the output matter beyond "it connects":
 *
 * Logging is off. `access` is set to `none` and the level never goes below
 * `warning`. The access log records every destination, which is exactly the
 * browsing history the privacy model forbids storing. It is disabled here, at
 * the only place that generates configuration, so no code path can turn it on.
 *
 * DNS resolves inside the tunnel. Proxied names are resolved remotely over
 * DoH, so the local resolver never sees which sites the user visits. Names on
 * the direct list are resolved locally on purpose: those services must see a
 * Russian address, and resolving them abroad defeats the split.
 */
object XrayConfigBuilder {

    const val SOCKS_PORT = 10808
    const val DNS_PORT = 10853

    /** Address the TUN advertises as DNS; captured and answered by the engine. */
    const val TUN_DNS_ADDRESS = "10.10.10.2"

    private const val REMOTE_DOH = "https://1.1.1.1/dns-query"
    private const val LOCAL_DNS = "223.5.5.5"

    fun build(profile: ConnectionProfile, policy: RoutingPolicy): String {
        require(profile.transportKind == ConnectionProfile.KIND_VLESS_REALITY) {
            "Unsupported transport kind: ${profile.transportKind}"
        }

        return JSONObject().apply {
            put("log", buildLog())
            put("dns", buildDns(policy))
            put("inbounds", buildInbounds())
            put("outbounds", buildOutbounds(profile))
            put("routing", buildRouting(policy))
        }.toString()
    }

    private fun buildLog(): JSONObject = JSONObject().apply {
        // Not a default that can be overridden elsewhere: this is the only
        // generator, so an access log cannot appear at runtime.
        put("access", "none")
        put("error", "")
        put("loglevel", "warning")
    }

    private fun buildDns(policy: RoutingPolicy): JSONObject {
        val servers = JSONArray()

        // Remote resolver first: it is the default for everything proxied.
        servers.put(REMOTE_DOH)

        // Local resolver, scoped to the names that must resolve to Russian
        // endpoints. Scoping matters: made global it would leak every lookup.
        if (policy.directDomains.isNotEmpty()) {
            servers.put(
                JSONObject().apply {
                    put("address", LOCAL_DNS)
                    put("port", 53)
                    put("domains", JSONArray(policy.directDomains))
                    put("skipFallback", true)
                },
            )
        }

        return JSONObject().apply {
            put("servers", servers)
            put("queryStrategy", "UseIPv4")
            put("disableCache", false)
        }
    }

    private fun buildInbounds(): JSONArray = JSONArray().apply {
        put(
            JSONObject().apply {
                put("tag", "socks-in")
                put("protocol", "socks")
                put("listen", "127.0.0.1")
                put("port", SOCKS_PORT)
                put(
                    "settings",
                    JSONObject().apply {
                        put("auth", "noauth")
                        put("udp", true)
                    },
                )
                put(
                    "sniffing",
                    JSONObject().apply {
                        // Sniffing recovers the hostname from the stream so that
                        // routing can act on names rather than addresses. Without
                        // it the direct list would only work by IP, and a Russian
                        // service behind a foreign CDN would be routed wrongly.
                        put("enabled", true)
                        put("destOverride", JSONArray(listOf("http", "tls", "quic")))
                        put("routeOnly", false)
                    },
                )
            },
        )
        put(
            JSONObject().apply {
                put("tag", "dns-in")
                put("protocol", "dokodemo-door")
                put("listen", "127.0.0.1")
                put("port", DNS_PORT)
                put(
                    "settings",
                    JSONObject().apply {
                        put("address", TUN_DNS_ADDRESS)
                        put("port", 53)
                        put("network", "tcp,udp")
                    },
                )
            },
        )
    }

    private fun buildOutbounds(profile: ConnectionProfile): JSONArray = JSONArray().apply {
        put(
            JSONObject().apply {
                put("tag", "proxy")
                put("protocol", "vless")
                put(
                    "settings",
                    JSONObject().apply {
                        put(
                            "vnext",
                            JSONArray().put(
                                JSONObject().apply {
                                    put("address", profile.host)
                                    put("port", profile.port)
                                    put(
                                        "users",
                                        JSONArray().put(
                                            JSONObject().apply {
                                                put("id", profile.credentialUuid)
                                                put("encryption", "none")
                                                put("flow", profile.flow)
                                            },
                                        ),
                                    )
                                },
                            ),
                        )
                    },
                )
                put(
                    "streamSettings",
                    JSONObject().apply {
                        put("network", "tcp")
                        put("security", "reality")
                        put(
                            "realitySettings",
                            JSONObject().apply {
                                put("serverName", profile.serverName)
                                put("publicKey", profile.publicKey)
                                put("shortId", profile.shortId)
                                put("fingerprint", profile.fingerprint)
                                put("show", false)
                            },
                        )
                    },
                )
            },
        )
        put(
            JSONObject().apply {
                put("tag", "direct")
                put("protocol", "freedom")
                put("settings", JSONObject().apply { put("domainStrategy", "UseIP") })
            },
        )
        put(
            JSONObject().apply {
                put("tag", "block")
                put("protocol", "blackhole")
            },
        )
        put(
            JSONObject().apply {
                put("tag", "dns-out")
                put("protocol", "dns")
            },
        )
    }

    private fun buildRouting(policy: RoutingPolicy): JSONObject {
        val rules = JSONArray()

        // DNS first: queries arriving on the dedicated inbound are answered by
        // the engine, not forwarded as ordinary traffic.
        rules.put(
            JSONObject().apply {
                put("type", "field")
                put("inboundTag", JSONArray(listOf("dns-in")))
                put("outboundTag", "dns-out")
            },
        )

        // Direct by geography and by private ranges.
        if (policy.directGeoRules.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("ip", JSONArray(policy.directGeoRules))
                    put("outboundTag", "direct")
                },
            )
        }

        // Direct by name, for services that must be reached from a Russian
        // address even when their addresses say otherwise.
        if (policy.directDomains.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("domain", JSONArray(policy.directDomains))
                    put("outboundTag", "direct")
                },
            )
        }

        return JSONObject().apply {
            // Names are resolved before matching so that a direct-listed name
            // behind a foreign address still goes direct.
            put("domainStrategy", "IPIfNonMatch")
            put("rules", rules)
        }
    }
}
