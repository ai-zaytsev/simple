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

    /** Port QUIC travels on, refused so that browsers fall back to TCP. */
    private const val QUIC_PORT = 443

    /** Port DNS actually travels on, as seen by routing. */
    private const val DNS_PORT_WIRE = 53

    /** Address the TUN advertises as DNS; captured and answered by the engine. */
    const val TUN_DNS_ADDRESS = "10.10.10.2"

    private const val REMOTE_DOH = "https://1.1.1.1/dns-query"

    /** A second provider, so one stalling resolver is not every name on the device. */
    private const val REMOTE_DOH_ALTERNATE = "https://8.8.8.8/dns-query"

    /** Addresses of the remote resolvers, for the rule that keeps them in the tunnel. */
    private val REMOTE_RESOLVER_IPS = listOf("1.1.1.1", "8.8.8.8")

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

        // Never below warning, with or without a file.
        //
        // This used to be `info` whenever a diagnostic file was wanted, and the
        // comment here admitted the cost and promised to remove it later. Later
        // arrived by way of an exported session log that named every site the
        // phone had visited - advertising endpoints, a video CDN, a telemetry
        // host - and then travelled off the device as an attachment.
        //
        // A phone writing its owner's browsing history to disk is the same
        // record the service is forbidden to keep, in the one place where
        // nobody is auditing. It does not become acceptable for being local:
        // it is more exportable there, not less.
        //
        // The diagnostic loss is real and smaller than it looks. What the
        // engine says at info is mostly which address it dialled, which is
        // exactly the part that must not be written; what actually diagnoses a
        // failure is the application's own narration in SessionLog, which names
        // endpoints, transports and outcomes and never names a destination.
        put("loglevel", if (errorLogPath == null) "warning" else "info")
        put("error", errorLogPath ?: "")
    }

    private fun buildDns(policy: RoutingPolicy): JSONObject {
        val servers = JSONArray()

        // Both resolvers speak over TCP, and that is the point.
        //
        // A session log from a device settled it. Queries sent as plain UDP
        // through the tunnel were answered for about a second after the session
        // was created and never again: the session was established twice in two
        // minutes, died both times, and was never rebuilt, so the engine went
        // on writing into a channel nobody was reading. Twenty-three answers
        // out of a hundred and sixty. Queries over the encrypted resolver, which
        // is carried by TCP, came back in 54 ms on the same tunnel at the same
        // time.
        //
        // So plain UDP is gone from the tunnel entirely. Two providers rather
        // than one, because a single resolver that stalls is still every name
        // on the device.
        servers.put(REMOTE_DOH)
        servers.put(REMOTE_DOH_ALTERNATE)

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

        // QUIC is refused, which sends browsers back to TCP.
        //
        // The tunnel is a TCP stream, and every UDP session crossing it opens
        // its own connection to the node. A phone browsing normally opens
        // hundreds: a device log showed 967 UDP packets in under two minutes,
        // against a node with 512 MB of memory, and connections through the
        // tunnel were being lost wholesale - "http2: client connection lost"
        // on a resolver that had answered in 54 ms minutes earlier.
        //
        // Refusing it is better than dropping it silently: a refused QUIC
        // attempt makes the browser fall back to TCP immediately, while a
        // black hole makes it wait for a timeout first. Carrying QUIC over a
        // TCP tunnel is a poor trade in any case, since a stall in the outer
        // stream stalls every stream inside it.
        rules.put(
            JSONObject().apply {
                put("type", "field")
                put("network", "udp")
                put("port", QUIC_PORT)
                put("outboundTag", "block")
            },
        )

        // The remote resolvers belong in the tunnel, said explicitly rather
        // than left to the default. They are the destinations whose exposure
        // would reveal every site visited, so it must not depend on a rule
        // further down happening not to match them.
        rules.put(
            JSONObject().apply {
                put("type", "field")
                put("ip", JSONArray(REMOTE_RESOLVER_IPS))
                put("outboundTag", "proxy")
            },
        )

        // The order below is the product rule, and each block exists because
        // the one after it would otherwise decide wrongly.

        // Straight out, whatever else matches. First because these are the
        // cases somebody has looked at and decided: a bank that refuses
        // foreign addresses, a service that must be reached from Russia. A
        // rule further down deciding otherwise would be a rule overriding
        // somebody's deliberate answer with a guess about geography.
        val directIPs = policy.directIPRules
        if (directIPs.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("ip", JSONArray(directIPs))
                    put("outboundTag", "direct")
                },
            )
        }
        if (policy.directDomains.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("domain", JSONArray(policy.directDomains))
                    put("outboundTag", "direct")
                },
            )
        }

        // Through the tunnel, whatever else matches. Second, so an explicit
        // "direct" still wins, and ahead of geography, so a foreign service
        // with a server in Russia is still reached through the tunnel - which
        // is the whole point of naming it.
        if (policy.proxyDomains.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("domain", JSONArray(policy.proxyDomains))
                    put("outboundTag", "proxy")
                },
            )
        }
        if (policy.proxyIPs.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("ip", JSONArray(policy.proxyIPs))
                    put("outboundTag", "proxy")
                },
            )
        }

        // Geography, for everything nobody has named. Last of the rules,
        // because it is the guess: a Russian address usually means a Russian
        // service, and usually is not always.
        val russia = policy.russiaIPRules
        if (russia.isNotEmpty()) {
            rules.put(
                JSONObject().apply {
                    put("type", "field")
                    put("ip", JSONArray(russia))
                    put("outboundTag", "direct")
                },
            )
        }

        // Everything else goes through the tunnel by falling off the end of
        // this list, which is what the default outbound is for.
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
