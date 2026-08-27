package download.simplevpn.plan

import org.json.JSONObject

/**
 * One way of reaching the Control Plane.
 *
 * Several exist because one is a single point of recovery, and a product whose
 * settings can only arrive down one path stops working the day that path is
 * blocked - with no way to fix it except a new build, which is slower than any
 * block.
 *
 * They differ deliberately, and each fails for a different reason:
 *
 *  - **name** is the ordinary path, and the first to go when a domain is
 *    blocked or its answers are poisoned.
 *  - **address** carries the name only inside the handshake and asks no
 *    resolver at all, so nothing done to DNS touches it. It falls with the
 *    machine's address.
 *  - **edge** is a different machine in a different country, at a different
 *    provider, on a domain from a different registrar, which forwards the API
 *    inward. It shares no failure with either of the others.
 *
 * None of this is secret. An adversary reads the descriptor like anybody else;
 * what it buys is not concealment but the speed of replacing an entry.
 */
data class Entry(
    val kind: Kind,
    val host: String,
    val port: Int,
    /** Presented in the handshake when [host] is an address. */
    val serverName: String,
    /** Where an edge forwards from; empty for the others. */
    val pathPrefix: String,
    val weight: Int,
) {

    enum class Kind { NAME, ADDRESS, EDGE }

    /** Where a request to this entry goes. */
    val baseUrl: String
        get() = "https://$host" + (if (port == 443) "" else ":$port") + pathPrefix

    /** The name to expect in the certificate, which is not always the host. */
    val expectedName: String
        get() = serverName.ifBlank { host }

    companion object {

        /** What the build knows before any descriptor has been fetched. */
        val SEED = listOf(
            Entry(
                kind = Kind.NAME,
                host = "simple-syncbridge.download",
                port = 443,
                serverName = "",
                pathPrefix = "",
                weight = 100,
            ),
            // The same machine reached without a resolver. In the build rather
            // than only in the descriptor, because a first installation has no
            // descriptor and this is the entry that survives a blocked or
            // poisoned resolver.
            //
            // A seed entry ages: it stays in old installations for ever. That
            // is why it names the address that is cheapest to defend, not the
            // most valuable asset.
            Entry(
                kind = Kind.ADDRESS,
                host = "185.9.26.52",
                port = 443,
                serverName = "simple-syncbridge.download",
                pathPrefix = "",
                weight = 80,
            ),
        )

        fun parse(json: JSONObject): Entry? {
            val kind = when (json.optString("kind")) {
                "https-direct" -> Kind.NAME
                "https-ip" -> Kind.ADDRESS
                "https-edge" -> Kind.EDGE
                // Not an error: a kind this build does not know about. Ignoring
                // it lets the server introduce one before every installation
                // understands it.
                else -> return null
            }

            val host = json.optString("host")
            if (host.isBlank()) return null

            return Entry(
                kind = kind,
                host = host,
                port = json.optInt("port", 443),
                serverName = json.optString("server_name", ""),
                pathPrefix = json.optString("path_prefix", ""),
                weight = json.optInt("weight", 100).coerceAtLeast(1),
            )
        }

        /**
         * The order to try entries in: random, weighted.
         *
         * Random rather than fixed because one order for every client is a
         * signature, and it puts the whole installed base on whichever entry
         * happens to be first. Weighted so that a cheap entry is usually tried
         * before an expensive one without ever being the only one tried.
         */
        fun order(entries: List<Entry>, random: (Int) -> Int): List<Entry> {
            val remaining = entries.toMutableList()
            val ordered = mutableListOf<Entry>()

            while (remaining.isNotEmpty()) {
                val total = remaining.sumOf { it.weight }
                var ticket = random(total)
                var chosen = remaining.lastIndex
                for ((index, entry) in remaining.withIndex()) {
                    ticket -= entry.weight
                    if (ticket < 0) {
                        chosen = index
                        break
                    }
                }
                ordered.add(remaining.removeAt(chosen))
            }
            return ordered
        }
    }
}
