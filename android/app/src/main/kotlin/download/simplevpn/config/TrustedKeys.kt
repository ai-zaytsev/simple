package download.simplevpn.config

import android.util.Base64

/**
 * Public keys the application will accept a signature from.
 *
 * This is the whole trust anchor of the product. Everything the server tells
 * the client - which node to use, which routing to apply, whether to stop
 * connecting at all - is believed because it carries a signature from one of
 * these keys and for no other reason. A build that trusts the wrong key here
 * trusts whoever holds it.
 *
 * There are two, and that is deliberate. A single key cannot be rotated: taking
 * it out of service would require every installation to update first, and an
 * update reaches users over days while a compromised key is a problem in
 * minutes. With two, one can be retired while the other keeps signing, and the
 * replacement travels in the next release without urgency.
 *
 * The private halves never existed in this repository. They were generated
 * once, written straight into the build system's secret store, and the local
 * copies destroyed; see docs/architecture/secrets-model.md.
 *
 * `keyId` in a signed document selects which of these to verify against. An
 * unknown id is a rejected document, never a document accepted unverified.
 */
object TrustedKeys {

    private val keys: Map<String, String> = mapOf(
        "cp-2026-08-a" to "nNfEd8mAhLVO+nAf4fVQ6gQi4kPwibATnrNcPeqKFIU=",
        "cp-2026-08-b" to "gkvDi0SrZd3JfmIMHvWPcOQoEngR39HqpWC6GCwi5BI=",
    )

    /** Raw 32 bytes for the given id, or null when the id is not trusted. */
    fun publicKey(keyId: String): ByteArray? {
        val encoded = keys[keyId] ?: return null
        return try {
            Base64.decode(encoded, Base64.NO_WRAP).takeIf { it.size == KEY_SIZE }
        } catch (t: Throwable) {
            // A key that cannot be decoded is not a key. Returning null makes
            // the document unverifiable, which is the correct outcome: the
            // alternative is trusting bytes nobody could parse.
            null
        }
    }

    /** Ed25519 public keys are exactly this long; anything else is not one. */
    private const val KEY_SIZE = 32
}
