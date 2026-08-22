package download.simplevpn.config

/**
 * Which traffic bypasses the tunnel.
 *
 * The product requirement is that anything needing a Russian address goes
 * direct and everything else goes through the proxy, without the user choosing
 * anything. That splits into two layers, because neither alone is sufficient.
 *
 * Layer one, [directPackages]: whole applications are excluded at the OS level.
 * Banking and government apps pin certificates, check the exit address, or
 * simply refuse to work from abroad. Routing their domains is unreliable
 * because those apps talk to many hosts; excluding the app is exact.
 *
 * Layer two, [directDomains] and [directGeoRules]: web traffic and everything
 * inside apps that stay in the tunnel.
 *
 * Both lists are data, not code. In the finished product they arrive from the
 * Control Plane and change without an app release. The values here are a
 * starting set for the slice.
 */
data class RoutingPolicy(
    val directPackages: List<String>,
    val directDomains: List<String>,
    val directGeoRules: List<String>,
) {
    companion object {

        /**
         * Deliberately short. A long guessed list creates false confidence:
         * every entry here should eventually come from measurement, not from
         * memory.
         */
        val DEFAULT = RoutingPolicy(
            directPackages = listOf(
                "ru.sberbankmobile",
                "ru.vtb24.mobilebanking.android",
                "ru.alfabank.mobile.android",
                "ru.tinkoff.sme",
                "com.idamob.tinkoff.android",
                "ru.gosuslugi.app",
                "ru.nalog.fl",
            ),
            directDomains = listOf(
                "domain:gosuslugi.ru",
                "domain:nalog.ru",
                "domain:sberbank.ru",
                "domain:vtb.ru",
                "domain:alfabank.ru",
                "domain:tinkoff.ru",
                "domain:mos.ru",
                "domain:yandex.ru",
                "domain:mail.ru",
                "domain:vk.com",
                "domain:ozon.ru",
                "domain:wildberries.ru",
                "domain:avito.ru",
                "domain:rt.ru",
                "domain:2gis.ru",
            ),
            directGeoRules = listOf(
                "geoip:private",
                "geoip:ru",
            ),
        )
    }
}
