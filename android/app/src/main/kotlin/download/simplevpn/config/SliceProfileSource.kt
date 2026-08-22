package download.simplevpn.config

import android.content.Context
import android.util.Log
import org.json.JSONObject

/**
 * Where the connection profile comes from during this vertical slice.
 *
 * In the finished product the profile is a field of the signed connection plan
 * issued by the Control Plane, which does not exist yet. Until then it is read
 * from an asset that is not part of the repository.
 *
 * The asset is deliberately absent from version control: it carries a working
 * credential and a node address, and a public repository is not a place for
 * either. A build without it produces an application that says it has no
 * endpoint, which is true, rather than one that fails obscurely at connect time.
 *
 * This whole class disappears when the plan API lands. It is the one place
 * where the client holds configuration it did not receive from the server, and
 * keeping it in a single file is what makes removing it a deletion rather than
 * a refactor.
 */
object SliceProfileSource {

    private const val TAG = "SliceProfileSource"
    private const val ASSET_NAME = "slice-profile.json"

    sealed interface Result {
        data class Available(val profile: ConnectionProfile) : Result
        data class Missing(val reason: String) : Result
    }

    fun load(context: Context): Result {
        val raw = try {
            context.assets.open(ASSET_NAME).bufferedReader().use { it.readText() }
        } catch (t: Throwable) {
            Log.i(TAG, "no bundled profile: ${t.message}")
            return Result.Missing("This build has no endpoint configured")
        }

        return try {
            val json = JSONObject(raw)
            Result.Available(
                ConnectionProfile(
                    alias = json.optString("alias", "slice"),
                    host = json.getString("host"),
                    port = json.getInt("port"),
                    transport = parseTransport(json),
                ),
            )
        } catch (t: Throwable) {
            Log.w(TAG, "bundled profile is malformed", t)
            Result.Missing("Endpoint configuration is malformed")
        }
    }

    private fun parseTransport(json: JSONObject): TransportParams {
        val kind = json.optString("transport_kind", TransportParams.VlessWsTls.KIND)
        val fingerprint = json.optString("fingerprint", "chrome")

        return when (kind) {
            TransportParams.VlessWsTls.KIND -> {
                val serverName = json.getString("server_name")
                TransportParams.VlessWsTls(
                    credentialUuid = json.getString("credential_uuid"),
                    path = json.getString("path"),
                    serverName = serverName,
                    // Defaults to the certificate name: they differ only in
                    // setups this slice does not have.
                    hostHeader = json.optString("host_header", serverName),
                    fingerprint = fingerprint,
                )
            }

            TransportParams.VlessReality.KIND -> TransportParams.VlessReality(
                credentialUuid = json.getString("credential_uuid"),
                flow = json.optString("flow", "xtls-rprx-vision"),
                serverName = json.getString("server_name"),
                publicKey = json.getString("public_key"),
                shortId = json.optString("short_id", ""),
                fingerprint = fingerprint,
            )

            else -> throw IllegalArgumentException("Unsupported transport kind: $kind")
        }
    }
}
