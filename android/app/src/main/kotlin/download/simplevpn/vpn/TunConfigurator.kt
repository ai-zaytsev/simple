package download.simplevpn.vpn

import android.content.pm.PackageManager
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log
import download.simplevpn.config.RoutingPolicy
import download.simplevpn.config.XrayConfigBuilder

/**
 * Turns a routing policy into an established TUN interface.
 *
 * Two decisions here are load-bearing and easy to get wrong.
 *
 * The application excludes itself. Without that, packets the engine sends to
 * the VPN server are themselves routed into the TUN, which feeds them back to
 * the engine: the loop that makes a tunnel appear to hang. Excluding the own
 * package is the exact fix and the reason "no VPN loop" holds.
 *
 * IPv6 is routed into the tunnel rather than left alone. If IPv6 were not
 * claimed, a device with working IPv6 would reach sites outside the tunnel
 * while the user believes everything is protected. Claiming it means IPv6
 * either goes through the tunnel or fails, and failing visibly beats leaking
 * silently.
 */
class TunConfigurator(private val service: VpnService) {

    fun establish(policy: RoutingPolicy): ParcelFileDescriptor? {
        val builder = service.Builder()
            .setSession(SESSION_NAME)
            .setMtu(MTU)
            .addAddress(TUN_ADDRESS_V4, TUN_PREFIX_V4)
            .addRoute(DEFAULT_ROUTE_V4, 0)
            .addAddress(TUN_ADDRESS_V6, TUN_PREFIX_V6)
            .addRoute(DEFAULT_ROUTE_V6, 0)
            .addDnsServer(XrayConfigBuilder.TUN_DNS_ADDRESS)

        excludeSelf(builder)
        excludeDirectPackages(builder, policy.directApps)

        return try {
            builder.establish()
        } catch (t: Throwable) {
            // establish() throws when consent was revoked between the check and
            // this call, and returns null when the service is not prepared.
            Log.w(TAG, "establish failed", t)
            null
        }
    }

    private fun excludeSelf(builder: VpnService.Builder) {
        try {
            builder.addDisallowedApplication(service.packageName)
        } catch (e: PackageManager.NameNotFoundException) {
            // Cannot happen for the own package, but a silent failure here
            // would mean a routing loop, so it is not swallowed quietly.
            Log.e(TAG, "could not exclude own package, tunnel would loop", e)
            throw IllegalStateException("Own package could not be excluded from the tunnel", e)
        }
    }

    private fun excludeDirectPackages(builder: VpnService.Builder, packages: List<String>) {
        var excluded = 0
        for (name in packages) {
            try {
                builder.addDisallowedApplication(name)
                excluded++
            } catch (_: PackageManager.NameNotFoundException) {
                // Expected: the list covers applications the user may not have
                // installed. A missing one is not an error.
            }
        }
        Log.i(TAG, "excluded $excluded of ${packages.size} direct applications")
    }

    companion object {
        /** The packet bridge must be given the same value the interface was built with. */
        const val MTU = 1500

        private const val TAG = "TunConfigurator"
        private const val SESSION_NAME = "Simple VPN"

        private const val TUN_ADDRESS_V4 = "10.10.10.1"
        private const val TUN_PREFIX_V4 = 32
        private const val DEFAULT_ROUTE_V4 = "0.0.0.0"

        private const val TUN_ADDRESS_V6 = "fd00:1:1:1::1"
        private const val TUN_PREFIX_V6 = 128
        private const val DEFAULT_ROUTE_V6 = "::"
    }
}
