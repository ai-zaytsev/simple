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

    /** Port DNS actually travels on, as seen by routing. */
    private const val DNS_PORT_WIRE = 53

    /** Address the TUN advertises as DNS; captured and answered by the engine. */
    const val TUN_DNS_ADDRESS = "10.10.10.2"

    private const val REMOTE_DOH = "https://1.1.1.1/dns-query"

    /** Same resolver, plain transport, used when the encrypted one answers nothing. */
    private const val REMOTE_PLAIN = "1.1.1.1"

    /** Address of the remote resolver, for the rule that keeps it in the tunnel. */
    private const val REMOTE_RESOLVER_IP = "1.1.1.1"

    /**
     * Resolver for the direct list only. A Russian one on purpose: those names
     * must resolve the way they resolve for a user in Russia, and a resolver
     * abroad hands back addresses chosen for somebody else's location.
     */
    private const val LOCAL_DNS = "77.88.8.8"

    fun build(
        profile: ConnectionProfile,
        policy: RoutingPolicy,
        errorLogPath: String? = null,
    ): String {
        return JSONObject().apply {
            put("log", buildLog(errorLogPath))
            put("dns", buildDns(policy))
            put("inbounds", buildInbounds())
            put("outbounds", buildOutbounds(profile))
            put("routing", buildRouting(policy))
        }.toString()
    }

    private fun buildLog(errorLogPath: String?): JSONObject = JSONObject().apply {
        // Not a default that can be overridden elsewhere: this is the only
        // generator, so an access log cannot appear at runtime.
        put("access", "none")

        if (errorLogPath == null) {
            put("error", "")
            put("loglevel", "warning")
        } else {
            // Diagnostic path, used while the slice is being brought up on a
            // real device. The engine reports a refused or stalled outbound at
            // info level, so warning hides exactly the line worth having.
            //
            // The cost is real and is why this is not the default: at info
            // level the engine names the destinations it dials, which is the
            // browsing history the privacy model forbids keeping. The file
            // lives in the application's private storage, is truncated at every
            // start, and this level goes away with the slice.
            put("error", errorLogPath)
            put("loglevel", "info")
        }
    }

    private fun buildDns(policy: RoutingPolicy): JSONObject {
        val servers = JSONArray()

        // Remote resolver first: it is the default for everything proxied.
        servers.put(REMOTE_DOH)

        // The same resolver in plain form, as a fallback.
        //
        // A device reported every lookup failing with "record not found" while
        // the tunnel itself carried traffic. One resolver that answers nothing
        // is a total outage: names never resolve, so nothing opens, even though
        // the connection is fine. A second entry against the same address but
        // a different transport means one broken transport costs latency
        // instead of everything. Both travel inside the tunnel, forced there by
        // a routing rule, so the local network sees neither.
        servers.put(REMOTE_PLAIN)

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
                put("settings", buildVlessSettings(profile))
                put("streamSettings", buildStreamSettings(profile.transport))
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

    private fun buildVlessSettings(profile: ConnectionProfile): JSONObject {
        val user = JSONObject().apply {
            put("id", profile.transport.credentialUuid)
            put("encryption", "none")
            // flow belongs to REALITY only. Sending it with WebSocket is not
            // merely useless: the server rejects the handshake.
            if (profile.transport is TransportParams.VlessReality) {
                put("flow", profile.transport.flow)
            }
        }

        return JSONObject().apply {
            put(
                "vnext",
                JSONArray().put(
                    JSONObject().apply {
                        put("address", profile.host)
                        put("port", profile.port)
                        put("users", JSONArray().put(user))
                    },
                ),
            )
        }
    }

    /**
     * The only place that knows how each transport is shaped.
     *
     * An unknown kind is impossible here because the type is sealed: adding a
     * transport forces this expression to be updated, which is the point of
     * modelling transport as a closed set rather than a string.
     */
    private fun buildStreamSettings(transport: TransportParams): JSONObject = when (transport) {

        is TransportParams.VlessWsTls -> JSONObject().apply {
            put("network", "ws")
            put("security", "tls")
            put(
                "wsSettings",
                JSONObject().apply {
                    put("path", transport.path)
                    // The Host header must match the certificate name, because
                    // Nginx routes on it and any mismatch is a visible anomaly
                    // in an otherwise ordinary HTTPS request.
                    put("headers", JSONObject().apply { put("Host", transport.hostHeader) })
                },
            )
            put(
                "tlsSettings",
                JSONObject().apply {
                    put("serverName", transport.serverName)
                    put("fingerprint", transport.fingerprint)
                    // Never true. Accepting an invalid certificate would turn a
                    // detectable interception into a silent one.
                    put("allowInsecure", false)
                    // http/1.1 only, and the omission of h2 is the point.
                    //
                    // The tunnel is carried by a WebSocket, and a WebSocket is
                    // opened by an HTTP/1.1 upgrade. Offering h2 lets the
                    // server choose it - ours does, since it serves an ordinary
                    // site over HTTP/2 - and the upgrade request then arrives
                    // on a connection where the server expects HTTP/2 frames.
                    // Neither side errors: each waits for the other, and the
                    // connection hangs after a handshake that looked perfect.
                    put("alpn", JSONArray(listOf("http/1.1")))
                },
            )
        }

        is TransportParams.VlessReality -> JSONObject().apply {
            put("network", "tcp")
            put("security", "reality")
            put(
                "realitySettings",
                JSONObject().apply {
                    put("serverName", transport.serverName)
                    put("publicKey", transport.publicKey)
                    put("shortId", transport.shortId)
                    put("fingerprint", transport.fingerprint)
                    put("show", false)
                },
            )
        }
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

        // And by port, which is the rule that actually carries the device.
        //
        // The interface advertises a DNS address inside the tunnel's own
        // subnet. The packet bridge forwards those queries to the engine as
        // ordinary traffic to that address, without knowing it is DNS, so they
        // never reach the inbound above. The private-range rule below would
        // then send them out as direct traffic to an address that exists
        // nowhere, and every lookup would time out: a tunnel that connects,
        // reports success and loads nothing.
        //
        // Matching on the port instead catches them wherever they arrive from,
        // and this rule must stay ahead of the address rules for that reason.
        // Scoped to traffic that arrives from the device. Unscoped, this rule
        // also catches the engine's own queries to the resolvers above, sending
        // them back to the resolver that issued them: a loop that answers
        // nothing and is invisible in any counter.
        rules.put(
            JSONObject().apply {
                put("type", "field")
                put("inboundTag", JSONArray(listOf("socks-in")))
                put("port", DNS_PORT_WIRE)
                put("outboundTag", "dns-out")
            },
        )

        // The remote resolver belongs in the tunnel, said explicitly rather
        // than left to the default. It is the one destination whose exposure
        // would reveal every site visited, so it must not depend on a rule
        // further down happening not to match it.
        rules.put(
            JSONObject().apply {
                put("type", "field")
                put("ip", JSONArray(listOf(REMOTE_RESOLVER_IP)))
                put("outboundTag", "proxy")
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
            // Addresses are matched as they arrive.
            //
            // Resolving every destination before matching it made each
            // connection wait on the resolver, so a resolver that answers
            // slowly or not at all stopped all traffic rather than just the
            // lookups. It is also unnecessary here: the device has already
            // resolved the name through this same engine, so the address rules
            // see a real address, while sniffing recovers the name for the
            // rules that match on names.
            put("domainStrategy", "AsIs")
            put("rules", rules)
        }
    }
}
