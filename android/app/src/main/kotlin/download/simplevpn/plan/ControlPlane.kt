package download.simplevpn.plan

/**
 * The canonical name of this service.
 *
 * No longer the only way to reach it - that is what [Entry] and [EntryBook] are
 * for, and a single address was the single point of recovery this product may
 * not have. What remains here is the name the service calls itself, used where
 * a name is wanted rather than a way in: the certificate to expect, and a
 * destination for the request that proves a tunnel carries traffic.
 *
 * A name works for that and would not work for a node. It is reached over the
 * phone's ordinary network or through an established tunnel, where a resolver
 * behaves normally. A node is dialled by the engine while the tunnel is being
 * built, when the resolver already points inside a tunnel that is not up yet -
 * which is why ADR-028 requires node addresses to be numeric.
 */
object ControlPlane {
    const val NAME = "simple-syncbridge.download"
    const val BASE_URL = "https://$NAME"
}
