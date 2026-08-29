package download.simplevpn.ui

import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The VIP offer on the screen, checked from the source.
 *
 * The Business Owner cannot exercise any of this by hand: the only account
 * that exists has been given VIP, so it sees the devices section and never the
 * offer. Every rule below is therefore unreachable from the application, which
 * is exactly why it is written down here.
 */
class VipOfferTest {

    private val screen = source("ui/VpnScreen.kt")
    private val client = source("plan/ControlPlaneClient.kt")

    @Test
    fun `the button is shown to a free account rather than hidden`() {
        // The chosen behaviour of the two the stage offered. Hiding it until
        // the wait is over means the offer is only ever seen by somebody who
        // opens the application on the right day.
        assertTrue(
            "the VIP button is not shown to an account that is not VIP",
            screen.contains("""else if (tier != null) {"""),
        )
        assertTrue(
            "there is no VIP button",
            screen.contains("VipButton("),
        )
    }

    @Test
    fun `nothing is shown until the service has answered`() {
        // An unknown standing draws neither corner. Guessing FREE would offer
        // VIP to somebody who has it; guessing VIP would show a devices
        // section that cannot work.
        assertTrue(
            "the corner is drawn before the standing is known",
            screen.contains("tier != null"),
        )
    }

    @Test
    fun `the phone does not decide when the wait ends`() {
        // The date is read for display and never compared with the phone's
        // clock. A device a week fast would otherwise announce that the wait
        // is over while the service went on refusing.
        assertTrue(
            "the application computes the wait itself",
            !screen.contains("System.currentTimeMillis() >") &&
                !screen.contains("Instant.now()"),
        )
        assertTrue(
            "the offer is not taken from the service's answer",
            screen.contains("standing?.mayBuy"),
        )
    }

    @Test
    fun `each refusal gets its own words`() {
        // Three answers, because they need three different things from the
        // reader: a date to come back on, nothing to do, or nothing at all.
        assertTrue("no wording for the wait", screen.contains("R.string.vip_wait"))
        assertTrue("no wording for sales being off", screen.contains("R.string.vip_closed"))
        assertTrue(
            "the reason from the service is never looked at",
            screen.contains("""standing?.whyNot == "too_soon""""),
        )
    }

    @Test
    fun `the standing arrives in one answer`() {
        // Tier and offer in one call. Split apart they would be two requests
        // that can disagree, and the disagreement shows as an offer to buy
        // something the account already has.
        assertTrue(
            "the client does not read the offer",
            client.contains("""json.optJSONObject("purchase")"""),
        )
        assertTrue(
            "the standing is not one value",
            client.contains("data class Standing("),
        )
    }

    private fun source(path: String): String {
        val file = java.io.File("src/main/kotlin/download/simplevpn/$path")
        assertTrue("cannot find $path", file.isFile)
        return file.readText()
    }
}
