package download.simplevpn.ui

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class VipPaymentTest {
    private val root = java.io.File("src/main")
    private val client = java.io.File(root, "kotlin/download/simplevpn/plan/ControlPlaneClient.kt").readText()
    private val screen = java.io.File(root, "kotlin/download/simplevpn/ui/VpnScreen.kt").readText()
    private val gradle = java.io.File("build.gradle.kts").readText()

    @Test
    fun `android sends only the server product identifier`() {
        assertTrue(client.contains("put(\"product_id\", productId)"))
        assertFalse(client.contains("put(\"amount"))
        assertFalse(client.contains("put(\"duration"))
    }

    @Test
    fun `checkout is an external https page`() {
        assertTrue(screen.contains("Intent.ACTION_VIEW"))
        assertTrue(screen.contains("uri.scheme != \"https\""))
    }

    @Test
    fun `return only rereads Core state`() {
        assertTrue(screen.contains("Lifecycle.Event.ON_RESUME"))
        assertTrue(client.contains("fun currentPayment()"))
        assertFalse(client.contains("setTier"))
        assertFalse(screen.contains("setTier"))
    }

    @Test
    fun `no mobile payment SDK or credential is in Android`() {
        val all = root.walkTopDown().filter { it.isFile }.joinToString("\n") { it.readText() } + gradle
        assertFalse(all.contains("YOOKASSA_TEST_SHOP_ID"))
        assertFalse(all.contains("YOOKASSA_TEST_SECRET_KEY"))
        assertFalse(all.contains("YOOKASSA_TEST_MOBILE_SDK_KEY"))
        assertFalse(all.lowercase().contains("yookassa"))
    }
}
