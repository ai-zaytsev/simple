package download.simplevpn.plan

import android.util.Log
import download.simplevpn.config.TrustedKeys
import org.json.JSONObject
import vpncore.Vpncore

/**
 * Opens an envelope from the Control Plane, or refuses to.
 *
 * Everything the server says arrives wrapped like this, and the only reason to
 * believe any of it is the signature. So the order here is not an
 * implementation detail: the signature is checked before the contents are read,
 * because a document that has not been verified is a document written by
 * whoever answered fastest.
 *
 * Verification happens in the native library rather than here. The platform
 * only gained an Ed25519 verifier at API level 33 and the minimum supported
 * level is 24; the Go runtime is already linked in for the tunnel and behaves
 * the same on every device.
 */
object SignedDocument {

    sealed interface Result {
        data class Trusted(val payload: JSONObject) : Result
        data class Rejected(val reason: String) : Result
    }

    /**
     * @param envelopeJson the raw response body, exactly as received
     */
    fun open(envelopeJson: String): Result {
        val envelope = try {
            JSONObject(envelopeJson)
        } catch (t: Throwable) {
            return Result.Rejected("envelope is not readable")
        }

        val algorithm = envelope.optString("alg")
        if (algorithm != ALGORITHM) {
            // An unexpected algorithm is not something to accommodate. The one
            // we accept is the one the keys are for.
            return Result.Rejected("unexpected algorithm")
        }

        val keyId = envelope.optString("key_id")
        val publicKey = TrustedKeys.publicKey(keyId)
            ?: return Result.Rejected("unknown signing key")

        val payloadB64 = envelope.optString("payload_b64")
        val signatureB64 = envelope.optString("sig_b64")
        if (payloadB64.isEmpty() || signatureB64.isEmpty()) {
            return Result.Rejected("envelope is incomplete")
        }

        try {
            // Throws when the signature does not match. Any failure here means
            // discard, never "proceed carefully".
            Vpncore.verifyDocument(payloadB64, signatureB64, encode(publicKey))
        } catch (t: Throwable) {
            Log.w(TAG, "signature rejected", t)
            return Result.Rejected("signature does not match")
        }

        return try {
            // Decoded on the native side so that what is parsed is byte for
            // byte what was verified.
            Result.Trusted(JSONObject(Vpncore.decodePayload(payloadB64)))
        } catch (t: Throwable) {
            Result.Rejected("document is not readable")
        }
    }

    private fun encode(key: ByteArray): String =
        android.util.Base64.encodeToString(key, android.util.Base64.NO_WRAP)

    private const val TAG = "SignedDocument"
    private const val ALGORITHM = "ed25519"
}
