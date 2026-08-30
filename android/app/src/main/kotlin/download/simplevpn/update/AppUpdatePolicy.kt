package download.simplevpn.update

import org.json.JSONObject
import java.net.URI

/** The version decision shared by every distribution channel. */
data class AppUpdatePolicy(
    val latestVersionCode: Int,
    val latestVersionName: String,
    val minSupportedVersionCode: Int,
    val channels: Map<String, Artifact>,
) {
    data class Artifact(val url: String, val sha256: String) {
        fun isValidDirectApk(): Boolean {
            val address = runCatching { URI(url) }.getOrNull() ?: return false
            return address.scheme == "https" &&
                !address.host.isNullOrBlank() &&
                address.userInfo == null &&
                address.fragment == null &&
                SHA256.matches(sha256)
        }
    }

    sealed interface Verdict {
        data object Current : Verdict
        data class Optional(val policy: AppUpdatePolicy, val artifact: Artifact?) : Verdict
        data class Required(val policy: AppUpdatePolicy, val artifact: Artifact?) : Verdict
    }

    fun verdict(currentVersionCode: Int, channel: String): Verdict {
        val artifact = channels[channel]
        return when {
            currentVersionCode < minSupportedVersionCode -> Verdict.Required(this, artifact)
            currentVersionCode < latestVersionCode -> Verdict.Optional(this, artifact)
            else -> Verdict.Current
        }
    }

    companion object {
        const val DIRECT_APK = "direct_apk"
        private val SHA256 = Regex("^[0-9a-f]{64}$")

        fun parse(root: JSONObject, legacyMinimum: Int): AppUpdatePolicy? {
            val update = root.optJSONObject("update")
                ?: return if (legacyMinimum > 0) {
                    // A Core from before this stage can still raise its stop
                    // line. It has no artifact to offer, so the client blocks
                    // honestly and keeps retrying config instead of inventing
                    // a download address.
                    AppUpdatePolicy(
                        latestVersionCode = legacyMinimum,
                        latestVersionName = "legacy",
                        minSupportedVersionCode = legacyMinimum,
                        channels = emptyMap(),
                    )
                } else {
                    null
                }

            val latest = update.optInt("latest_version_code", 0)
            val minimum = update.optInt("min_supported_version_code", 0)
            val name = update.optString("latest_version_name")
            if (latest < 1 || minimum < 1 || minimum > latest || name.isBlank()) return null

            val parsedChannels = mutableMapOf<String, Artifact>()
            val channels = update.optJSONObject("channels") ?: JSONObject()
            val keys = channels.keys()
            while (keys.hasNext()) {
                val key = keys.next()
                val item = channels.optJSONObject(key) ?: return null
                val artifact = Artifact(
                    url = item.optString("url"),
                    sha256 = item.optString("sha256"),
                )
                if (key == DIRECT_APK && !artifact.isValidDirectApk()) return null
                parsedChannels[key] = artifact
            }
            return AppUpdatePolicy(latest, name, minimum, parsedChannels)
        }
    }
}
