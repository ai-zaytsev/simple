package download.simplevpn.config

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What the engine on somebody's phone is allowed to write down.
 *
 * This exists because the answer was wrong in production and nothing noticed.
 * Whenever a diagnostic file was wanted the generator set the engine to `info`,
 * and at that level Xray names every address it dials. A phone was therefore
 * keeping its owner's complete browsing history in a file - which then left the
 * device as an attachment, because the application offers a button to send it.
 *
 * There is now a level that names destinations, and it is reachable only from
 * [XrayConfigBuilder.EngineLog.Trace], which exists only while a recording the
 * user started is running. These tests hold that arrangement in place: that
 * wanting a file is not wanting a history, and that only one of the three
 * settings can produce one.
 */
class EngineLoggingTest {

    private val profile = ConnectionProfile(
        alias = "n-test",
        host = "198.51.100.7",
        port = 443,
        transport = TransportParams.VlessWsTls(
            credentialUuid = "00000000-0000-4000-8000-000000000000",
            path = "/somewhere",
            serverName = "example.invalid",
            hostHeader = "example.invalid",
            fingerprint = "chrome",
        ),
    )

    private fun logSection(engineLog: XrayConfigBuilder.EngineLog): JSONObject =
        JSONObject(
            XrayConfigBuilder.build(profile, RoutingPolicy.UNTIL_A_PLAN_ARRIVES, engineLog),
        ).getJSONObject("log")

    /** Levels at which Xray writes the address of every connection it makes. */
    private val namesDestinations = setOf("debug", "info")

    @Test
    fun `the everyday settings never log at a level that names destinations`() {
        val everyday = listOf(
            XrayConfigBuilder.EngineLog.Off,
            XrayConfigBuilder.EngineLog.Errors("/data/data/download.simplevpn/files/engine.log"),
        )
        for (setting in everyday) {
            val level = logSection(setting).getString("loglevel")
            assertTrue(
                "$setting logs at $level, which names every destination the engine dials",
                level !in namesDestinations,
            )
        }
    }

    @Test
    fun `asking for a diagnostic file does not raise the level`() {
        // The trap that was actually sprung: the file and the level were one
        // decision, so wanting diagnostics meant accepting a browsing history.
        assertEquals(
            logSection(XrayConfigBuilder.EngineLog.Off).getString("loglevel"),
            logSection(XrayConfigBuilder.EngineLog.Errors("/tmp/engine.log")).getString("loglevel"),
        )
    }

    @Test
    fun `only a deliberate recording names destinations`() {
        // The other half of the arrangement. If this ever stops being true the
        // feature has become pointless: a recording that records nothing more
        // than the everyday log cannot answer the one question it exists for.
        val level = logSection(XrayConfigBuilder.EngineLog.Trace("/tmp/trace.log")).getString("loglevel")
        assertTrue(
            "a recording at $level would not name the destinations it is for",
            level in namesDestinations,
        )
    }

    @Test
    fun `a recording writes to its own file, never the everyday one`() {
        // Kept apart so that an ordinary support log sent a week later is not
        // still carrying whatever was recorded once.
        assertEquals("/tmp/trace.log", logSection(XrayConfigBuilder.EngineLog.Trace("/tmp/trace.log")).getString("error"))
        assertEquals("/tmp/engine.log", logSection(XrayConfigBuilder.EngineLog.Errors("/tmp/engine.log")).getString("error"))
        assertEquals("", logSection(XrayConfigBuilder.EngineLog.Off).getString("error"))
    }

    @Test
    fun `the access log is off in every setting, recording included`() {
        val all = listOf(
            XrayConfigBuilder.EngineLog.Off,
            XrayConfigBuilder.EngineLog.Errors("/tmp/engine.log"),
            XrayConfigBuilder.EngineLog.Trace("/tmp/trace.log"),
        )
        for (setting in all) {
            assertEquals(
                "$setting turned the access log on; that is a second copy in a second place to forget about",
                "none",
                logSection(setting).getString("access"),
            )
        }
    }
}
