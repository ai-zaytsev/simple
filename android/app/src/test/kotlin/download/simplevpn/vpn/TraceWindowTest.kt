package download.simplevpn.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class TraceWindowTest {

    @Test
    fun `a repeated start keeps the first deadline`() {
        val window = TraceWindow(limitMillis = 600_000L)

        val first = window.start(nowElapsedMillis = 1_000L)
        val repeated = window.start(nowElapsedMillis = 90_000L)

        assertTrue(first.newlyStarted)
        assertFalse(repeated.newlyStarted)
        assertEquals(601_000L, first.stopsAtElapsedMillis)
        assertEquals(first.stopsAtElapsedMillis, repeated.stopsAtElapsedMillis)
        assertTrue(window.isRunning)
    }

    @Test
    fun `stop closes the window once and a later start gets a new deadline`() {
        val window = TraceWindow(limitMillis = 600_000L)
        window.start(nowElapsedMillis = 1_000L)

        assertTrue(window.stop())
        assertFalse(window.stop())
        assertFalse(window.isRunning)

        val next = window.start(nowElapsedMillis = 2_000L)
        assertTrue(next.newlyStarted)
        assertEquals(602_000L, next.stopsAtElapsedMillis)
    }

    @Test(expected = IllegalArgumentException::class)
    fun `a recording window cannot be unbounded`() {
        TraceWindow(limitMillis = 0L)
    }

    @Test
    fun `the service cannot remove the deadline before rejecting a repeated start`() {
        val service = File(
            "src/main/kotlin/download/simplevpn/vpn/SimpleVpnService.kt",
        ).readText()
        val startBranch = service.substringAfter("if (on) {").substringBefore("} else {")

        val repeatedStartReturn = startBranch.indexOf("if (!started.newlyStarted) return")
        val callbackRemoval = startBranch.indexOf("watchHandler.removeCallbacks(stopTracing)")

        assertTrue("the repeated-start guard is missing", repeatedStartReturn >= 0)
        assertTrue("the start branch never owns a bounded callback", callbackRemoval >= 0)
        assertTrue(
            "a repeated start can remove the original deadline before it returns",
            repeatedStartReturn < callbackRemoval,
        )
    }
}
