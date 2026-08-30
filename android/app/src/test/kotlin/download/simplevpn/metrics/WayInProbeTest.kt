package download.simplevpn.metrics

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class WayInProbeTest {

    private fun source(relative: String): String =
        File("src/main/kotlin/download/simplevpn/$relative").readText()

    @Test
    fun `accepted sweep is bounded and clock rollback retries`() {
        val interval = 6 * 60 * 60 * 1_000L
        assertFalse(WayInProbe.due(1_000L, 1_000L + interval - 1L, false))
        assertTrue(WayInProbe.due(1_000L, 1_000L + interval, false))
        assertTrue(WayInProbe.due(1_000L, 999L, false))
        assertTrue(WayInProbe.due(1_000L, 1_001L, true))
    }

    @Test
    fun `every public entry is swept and tunnel attempts are excluded`() {
        val client = source("plan/ControlPlaneClient.kt")
        val sweep = client.substringAfter("fun probeWaysIn()").substringBefore("fun reportPlanFailure")
        assertTrue(sweep.contains("if (throughTunnel != null) return 0"))
        assertTrue(sweep.contains("for (entry in entries)"))
        assertTrue(sweep.contains("attempt(entry, \"/v1/config\""))
        assertTrue(client.contains("throughTunnel == null && path != REPORT_PATH"))
    }

    @Test
    fun `latest signed descriptor is learned before the sweep and sign-in forces it`() {
        val probe = source("metrics/WayInProbe.kt")
        assertTrue(probe.indexOf("refreshEntries()") < probe.indexOf("probeWaysIn()"))
        assertTrue(probe.contains("ServiceReport.drain()"))

        val activity = source("MainActivity.kt")
        assertTrue(activity.contains("WayInProbe.refresh(this)"))
        assertTrue(activity.contains("WayInProbe.refresh(this, force = true)"))
    }

    @Test
    fun `sign-in attempts report only our entry host`() {
        val auth = source("auth/AuthClient.kt")
        assertTrue(auth.contains("ServiceReport.probed(entry.host"))
        assertFalse(auth.contains("ServiceReport.probed(path"))

        val report = source("metrics/ServiceReport.kt")
        assertFalse(report.contains("visitedUrl"))
        assertFalse(report.contains("destinationUrl"))
    }
}
