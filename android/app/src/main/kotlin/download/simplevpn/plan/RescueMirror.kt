package download.simplevpn.plan

import android.util.Log
import java.net.HttpURLConnection
import java.net.URL

/**
 * The signed descriptor kept somewhere that is not ours.
 *
 * The last channel, and the only one that works for an installation that has
 * never connected. A device with a working plan can ask from inside its own
 * tunnel; a device installed today during a block has no plan, no tunnel and
 * no way in - unless the list of ways in can be fetched from somewhere the
 * block does not reach.
 *
 * A mirror is untrusted by design and that is not a compromise. The document is
 * signed, so a mirror can withhold it or serve an old one - the sequence rule
 * catches the second - but cannot change a word of it. Its contents are public
 * in any case: an adversary reads the descriptor as easily as anybody, and what
 * it buys is not concealment but the speed of replacing an entry.
 *
 * Nothing here is fetched unless every ordinary way in has already failed. In
 * the normal case this code never runs.
 */
object RescueMirror {

    /**
     * Where to look, in order.
     *
     * Deliberately somebody else's infrastructure, on somebody else's domain,
     * resolved by somebody else's DNS, in another jurisdiction. A mirror that
     * shared a failure with our own service would be no mirror at all.
     *
     * One is thin. The design asks for at least three in different
     * jurisdictions, and the others are an operational decision that costs
     * money or accounts; this one costs neither and is honest about being the
     * first of them rather than the whole set.
     */
    private val MIRRORS = listOf(
        "https://raw.githubusercontent.com/ai-zaytsev/simple/rescue/bootstrap.json",
    )

    /** @return the signed envelope as text, or null when no mirror answered. */
    fun fetch(timeoutMs: Int): String? {
        for (url in MIRRORS) {
            var connection: HttpURLConnection? = null
            try {
                connection = (URL(url).openConnection() as HttpURLConnection).apply {
                    requestMethod = "GET"
                    connectTimeout = timeoutMs
                    readTimeout = timeoutMs
                    // No header naming this application. A mirror is a public
                    // place; a request that identifies the product turns a
                    // static file into a list of who is looking for it.
                    setRequestProperty("accept", "application/json")
                }
                if (connection.responseCode == HttpURLConnection.HTTP_OK) {
                    val body = connection.inputStream.bufferedReader().readText()
                    if (body.isNotBlank()) return body
                }
                Log.i(TAG, "mirror answered ${connection.responseCode}")
            } catch (t: Throwable) {
                Log.i(TAG, "mirror unreachable: ${t.message}")
            } finally {
                connection?.disconnect()
            }
        }
        return null
    }

    private const val TAG = "RescueMirror"
}
