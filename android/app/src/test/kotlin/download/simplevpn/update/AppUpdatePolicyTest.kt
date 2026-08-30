package download.simplevpn.update

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.MessageDigest

class AppUpdatePolicyTest {
    private val hash = "a".repeat(64)

    private fun config(latest: Int = 5, minimum: Int = 2, url: String? = "https://simple-vpn.download/v5.apk") =
        JSONObject().apply {
            put("min_supported_app_version", minimum)
            put(
                "update",
                JSONObject().apply {
                    put("latest_version_code", latest)
                    put("latest_version_name", "0.5.0")
                    put("min_supported_version_code", minimum)
                    put(
                        "channels",
                        JSONObject().apply {
                            if (url != null) {
                                put(
                                    AppUpdatePolicy.DIRECT_APK,
                                    JSONObject().put("url", url).put("sha256", hash),
                                )
                            }
                        },
                    )
                },
            )
        }

    @Test
    fun `current optional and required share one boundary`() {
        val policy = AppUpdatePolicy.parse(config(), 2)!!
        assertTrue(policy.verdict(5, AppUpdatePolicy.DIRECT_APK) is AppUpdatePolicy.Verdict.Current)
        assertTrue(policy.verdict(2, AppUpdatePolicy.DIRECT_APK) is AppUpdatePolicy.Verdict.Optional)
        assertTrue(policy.verdict(1, AppUpdatePolicy.DIRECT_APK) is AppUpdatePolicy.Verdict.Required)
    }

    @Test
    fun `missing artifact does not make a forced update optional`() {
        val policy = AppUpdatePolicy.parse(config(url = null), 2)!!
        val verdict = policy.verdict(1, AppUpdatePolicy.DIRECT_APK) as AppUpdatePolicy.Verdict.Required
        assertNull(verdict.artifact)
    }

    @Test
    fun `old config still enforces its root minimum`() {
        val root = JSONObject().put("min_supported_app_version", 4)
        val policy = AppUpdatePolicy.parse(root, 4)!!
        assertTrue(policy.verdict(3, AppUpdatePolicy.DIRECT_APK) is AppUpdatePolicy.Verdict.Required)
    }

    @Test
    fun `insecure URL and malformed hash are rejected`() {
        assertNull(AppUpdatePolicy.parse(config(url = "http://simple-vpn.download/v5.apk"), 2))
        val malformed = config().also {
            it.getJSONObject("update").getJSONObject("channels")
                .getJSONObject(AppUpdatePolicy.DIRECT_APK).put("sha256", "ABC")
        }
        assertNull(AppUpdatePolicy.parse(malformed, 2))
    }

    @Test
    fun `actual APK digest must match all 256 bits`() {
        val digest = MessageDigest.getInstance("SHA-256").digest("apk".toByteArray())
        val expected = digest.joinToString("") { "%02x".format(it) }
        assertTrue(sha256Matches(digest, expected))
        assertFalse(sha256Matches(digest, "0".repeat(64)))
        assertEquals(64, expected.length)
    }
}
