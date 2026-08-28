package download.simplevpn.config

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * What the engine on somebody's phone is allowed to write down.
 *
 * This test exists because the answer was wrong in production and nothing
 * noticed. Whenever a diagnostic file was wanted the generator set the engine
 * to `info`, and at that level Xray names every address it dials. A phone was
 * therefore keeping its owner's complete browsing history in a file - which
 * then left the device as an attachment, because the application offers a
 * button to export it.
 *
 * The service is forbidden to keep that record. A copy on the phone is the same
 * record in the one place nobody audits, and it is more exportable there rather
 * than less. So the level is fixed here, and the fixing is checked.
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

    private fun logSection(errorLogPath: String?): JSONObject =
        JSONObject(
            XrayConfigBuilder.build(profile, RoutingPolicy.UNTIL_A_PLAN_ARRIVES, errorLogPath),
        ).getJSONObject("log")

    /** Levels at which Xray writes the address of every connection it makes. */
    private val namesDestinations = setOf("debug", "info")

    @Test
    fun `the engine never logs at a level that names destinations`() {
        for (path in listOf(null, "/data/data/download.simplevpn/files/engine.log")) {
            val log = logSection(path)
            val level = log.getString("loglevel")
            assertTrue(
                "loglevel $level names every destination the engine dials",
                level !in namesDestinations,
            )
        }
    }

    @Test
    fun `asking for a diagnostic file does not raise the level`() {
        // The trap that was actually sprung: the file and the level were one
        // decision, so wanting diagnostics meant accepting a browsing history.
        // They are separate now, and this is what says so.
        assertEquals(
            logSection(null).getString("loglevel"),
            logSection("/tmp/engine.log").getString("loglevel"),
        )
    }

    @Test
    fun `the access log is off however the engine is asked for`() {
        for (path in listOf(null, "/tmp/engine.log")) {
            assertEquals("none", logSection(path).getString("access"))
        }
    }
}
