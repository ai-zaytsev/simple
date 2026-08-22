package download.simplevpn.config

/**
 * Everything needed to establish one tunnel.
 *
 * In the finished product this is built from the signed connection plan issued
 * by the Control Plane, never chosen by the client (ADR-001). For this vertical
 * slice it is injected locally so the transport can be exercised before the
 * Control Plane exists.
 *
 * The shape mirrors the plan's transport envelope on purpose: [transportKind]
 * plus opaque parameters, so that a second transport is a new kind rather than
 * a new field on this class. That is invariant I-10.
 */
data class ConnectionProfile(
    val alias: String,
    val host: String,
    val port: Int,
    val transportKind: String,
    val credentialUuid: String,
    val flow: String,
    val serverName: String,
    val publicKey: String,
    val shortId: String,
    val fingerprint: String,
) {
    companion object {
        const val KIND_VLESS_REALITY = "vless-reality"
    }
}
