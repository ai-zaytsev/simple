package download.simplevpn.update

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

class AppUpdateContractTest {
    private fun source(relative: String): String =
        File("src/main/kotlin/download/simplevpn/$relative").readText()

    @Test
    fun `one build version is sent and enforced`() {
        assertTrue(source("plan/ControlPlaneClient.kt").contains("BuildConfig.VERSION_CODE"))
        assertTrue(source("vpn/SimpleVpnService.kt").contains("config.verdict(BuildConfig.VERSION_CODE)"))
        assertFalse(source("plan/ControlPlaneClient.kt").contains("const val APP_VERSION"))
        assertFalse(source("vpn/SimpleVpnService.kt").contains("const val APP_VERSION"))
    }

    @Test
    fun `hash is checked before Android installer session`() {
        val updater = source("update/DirectApkUpdater.kt")
        val check = updater.indexOf("sha256Matches")
        val session = updater.indexOf("installer.createSession")
        assertTrue(check >= 0)
        assertTrue(session > check)
        assertTrue(updater.contains("PackageInstaller.SessionParams"))
        assertFalse(updater.contains("ACTION_INSTALL_PACKAGE"))
        assertTrue(updater.contains("MAX_APK_BYTES"))
        assertTrue(updater.contains("instanceFollowRedirects = false"))
    }

    @Test
    fun `forced update cannot be dismissed and optional update can`() {
        val dialog = source("update/AppUpdateDialog.kt")
        assertTrue(dialog.contains("if (!state.required) onLater()"))
        assertTrue(dialog.contains("dismissButton = if (state.required)"))
        val activity = source("MainActivity.kt")
        assertTrue(activity.contains("AppUpdatePolicy.Verdict.Required"))
    }
}
