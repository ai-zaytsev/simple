package download.simplevpn.vpn

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** Holds the rule that no hidden product path can begin destination logging. */
class TraceEntryTest {

    private val sources = File("src/main/kotlin").walkTopDown()
        .filter { it.isFile && it.extension == "kt" }
        .toList()

    @Test
    fun `only the vpn service can construct destination logging`() {
        val constructors = sources.filter {
            it.readText().contains("XrayConfigBuilder.EngineLog.Trace(")
        }
        assertEquals(1, constructors.size)
        assertTrue(constructors.single().invariantSeparatorsPath.endsWith("vpn/SimpleVpnService.kt"))
    }

    @Test
    fun `only the warned screen can ask the controller to start a trace`() {
        val callers = sources.filter {
            it.readText().contains("VpnController.startTrace(")
        }
        assertEquals(1, callers.size)
        assertTrue(callers.single().invariantSeparatorsPath.endsWith("ui/VpnScreen.kt"))
    }

    @Test
    fun `another application cannot address the vpn service`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()
        val service = manifest.substringAfter("android:name=\".vpn.SimpleVpnService\"")
            .substringBefore("</service>")
        assertTrue(service.contains("android:exported=\"false\""))
    }
}

