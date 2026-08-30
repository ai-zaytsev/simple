package download.simplevpn.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class TraceRecoveryTest {

    @Test
    fun `a file left by a finished recording becomes ready`() {
        assertEquals(
            TraceState.Ready,
            restoredTraceState(TraceState.Idle, hasTrace = true),
        )
    }

    @Test
    fun `a missing file clears stale ready state`() {
        assertEquals(
            TraceState.Idle,
            restoredTraceState(TraceState.Ready, hasTrace = false),
        )
    }

    @Test
    fun `activity recreation cannot replace an active recording`() {
        val recording = TraceState.Recording(stopsAtElapsedMillis = 42L)
        assertSame(recording, restoredTraceState(recording, hasTrace = true))
        assertSame(recording, restoredTraceState(recording, hasTrace = false))
    }

    @Test
    fun `main activity restores rather than deletes a finished recording`() {
        val source = File("src/main/kotlin/download/simplevpn/MainActivity.kt").readText()
        assertTrue(source.contains("VpnController.restoreTrace(this)"))
        assertTrue(!source.contains("SessionLog.dropTrace"))
    }
}

