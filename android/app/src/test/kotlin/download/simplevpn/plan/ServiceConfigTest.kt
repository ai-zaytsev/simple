package download.simplevpn.plan

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The rules that can take the whole product off the air.
 *
 * Tested here rather than on a device because a decision this consequential
 * should be checkable without hardware, a network, or somebody willing to be
 * disconnected to find out whether it works.
 */
class ServiceConfigTest {

    private fun document(
        seq: Long = 1,
        kill: Boolean = false,
        minVersion: Int = 1,
    ) = JSONObject(
        """
        {"v":1,"seq":$seq,"issued_at":"2026-08-25T00:00:00Z",
         "min_supported_app_version":$minVersion,
         "kill_switch":{"enabled":$kill,"message_key":"stopped"},
         "refresh_after_s":900}
        """.trimIndent(),
    )

    @Test
    fun `an ordinary document lets this build run`() {
        val config = ServiceConfig.parse(document())!!
        assertEquals(ServiceConfig.Verdict.Allowed, config.verdict(appVersion = 1))
    }

    @Test
    fun `the kill switch stops this build`() {
        val config = ServiceConfig.parse(document(kill = true))!!
        val verdict = config.verdict(appVersion = 99) as ServiceConfig.Verdict.Stopped
        assertEquals(ServiceConfig.Stop.KILL_SWITCH, verdict.reason)
    }

    @Test
    fun `a build older than the minimum is stopped`() {
        val config = ServiceConfig.parse(document(minVersion = 5))!!
        val verdict = config.verdict(appVersion = 4) as ServiceConfig.Verdict.Stopped
        assertEquals(ServiceConfig.Stop.TOO_OLD, verdict.reason)
    }

    @Test
    fun `new update policy must agree with the legacy minimum`() {
        val payload = document(minVersion = 2)
        payload.put(
            "update",
            JSONObject(
                """{"latest_version_code":3,"latest_version_name":"0.3.0",
                    "min_supported_version_code":1,"channels":{}}""",
            ),
        )
        assertNull(ServiceConfig.parse(payload))
    }

    @Test
    fun `new update policy is available to channel executor`() {
        val payload = document(minVersion = 2)
        payload.put(
            "update",
            JSONObject(
                """{"latest_version_code":3,"latest_version_name":"0.3.0",
                    "min_supported_version_code":2,"channels":{}}""",
            ),
        )
        val config = ServiceConfig.parse(payload)!!
        assertEquals(3, config.update.latestVersionCode)
        assertEquals(2, config.update.minSupportedVersionCode)
    }

    @Test
    fun `when both apply the kill switch is the reason given`() {
        // Not a preference about wording. One of these is true of everybody
        // and the other blames the person's build; saying the second when the
        // first is what happened sends people to install an update that will
        // not help.
        val config = ServiceConfig.parse(document(kill = true, minVersion = 5))!!
        val verdict = config.verdict(appVersion = 1) as ServiceConfig.Verdict.Stopped
        assertEquals(ServiceConfig.Stop.KILL_SWITCH, verdict.reason)
    }

    @Test
    fun `a document without a kill switch does not invent one`() {
        val payload = JSONObject("""{"v":1,"seq":1,"min_supported_app_version":1}""")
        val config = ServiceConfig.parse(payload)!!
        assertFalse(config.killSwitch)
        assertEquals(ServiceConfig.Verdict.Allowed, config.verdict(appVersion = 1))
    }

    @Test
    fun `a document from a future version is not guessed at`() {
        val payload = JSONObject("""{"v":2,"seq":1,"kill_switch":{"enabled":true}}""")
        assertNull(ServiceConfig.parse(payload))
    }

    @Test
    fun `a higher number replaces the document in use`() {
        val current = ServiceConfig.parse(document(seq = 7))!!
        val candidate = ServiceConfig.parse(document(seq = 8))!!
        assertTrue(ServiceConfig.supersedes(candidate, current))
    }

    @Test
    fun `an older document cannot turn the switch off again`() {
        // The attack this exists for: record the configuration from before the
        // switch was thrown, serve it back, and the client goes on running.
        val current = ServiceConfig.parse(document(seq = 9, kill = true))!!
        val replayed = ServiceConfig.parse(document(seq = 4, kill = false))!!
        assertFalse(ServiceConfig.supersedes(replayed, current))
    }

    @Test
    fun `the same number is not newer`() {
        val current = ServiceConfig.parse(document(seq = 5))!!
        val same = ServiceConfig.parse(document(seq = 5, kill = false))!!
        assertFalse(ServiceConfig.supersedes(same, current))
    }

    @Test
    fun `the first document is always accepted`() {
        val candidate = ServiceConfig.parse(document(seq = 1))!!
        assertTrue(ServiceConfig.supersedes(candidate, null))
    }
}
