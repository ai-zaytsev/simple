package download.simplevpn.config

/**
 * Everything needed to establish one tunnel.
 *
 * In the finished product this is built from the signed connection plan issued
 * by the Control Plane, never chosen by the client (ADR-001). For this vertical
 * slice it is injected locally so the transport can be exercised before the
 * Control Plane exists.
 *
 * Transport parameters live in [transport] rather than as fields here. That is
 * invariant I-10: a second transport is a new kind, not a new column on this
 * class. The invariant paid for itself when REALITY stopped being the primary
 * transport (ADR-022) and WebSocket took its place: nothing outside this file
 * and the config builder had to change.
 */
data class ConnectionProfile(
    val alias: String,
    val host: String,
    val port: Int,
    val transport: TransportParams,
)

/**
 * Transport-specific parameters.
 *
 * Two kinds exist. WebSocket behind Nginx is primary: a request to the domain
 * serves a real site, and the tunnel lives on one dedicated path (ADR-022).
 * REALITY is the standby on a separate port (ADR-024), kept because a block of
 * one transport class should not take the service down.
 */
sealed interface TransportParams {

    val kind: String
    val credentialUuid: String

    /**
     * VLESS over WebSocket over TLS, terminated by Nginx on our own domain.
     *
     * [serverName] is the certificate name and the SNI. The client connects to
     * [ConnectionProfile.host], which is the node address, while presenting
     * this name: nodes of one group share a domain and each holds its own
     * certificate for it (ADR-023).
     */
    data class VlessWsTls(
        override val credentialUuid: String,
        val path: String,
        val serverName: String,
        val hostHeader: String,
        val fingerprint: String,
    ) : TransportParams {
        override val kind: String get() = KIND

        companion object {
            const val KIND = "vless-ws-tls"
        }
    }

    /** VLESS over REALITY. Standby transport on a non-standard port. */
    data class VlessReality(
        override val credentialUuid: String,
        val flow: String,
        val serverName: String,
        val publicKey: String,
        val shortId: String,
        val fingerprint: String,
    ) : TransportParams {
        override val kind: String get() = KIND

        companion object {
            const val KIND = "vless-reality"
        }
    }
}
