package download.simplevpn.plan

/**
 * The one address compiled into the build.
 *
 * A name rather than an address, and that is what makes the service movable:
 * putting it on another machine is a DNS change, not an update to every
 * installation.
 *
 * A name works here and would not work for a node. This is reached before any
 * tunnel exists, over the phone's ordinary network, where the system resolver
 * behaves normally. A node is dialled by the engine while the tunnel is being
 * established, when the resolver already points inside a tunnel that is not up
 * yet - which is why ADR-028 requires node addresses to be numeric.
 */
object ControlPlane {
    const val BASE_URL = "https://simple-syncbridge.download"
}
