package download.simplevpn.support

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * There is one letter to support, and the recording rides in it.
 *
 * The Business Owner asked for this in as many words: the recording goes out
 * the same way and with the same text as the support button, or it is saved to
 * the device - and nothing else. Mail is the one channel that has stayed
 * reachable from Russia; everything else we might have offered is blockable
 * from outside and has been blocked.
 *
 * "Same text" is held by construction rather than by care. Both intents are
 * filled by one private function, so the address, the subject and the body
 * cannot drift apart the next time either is edited - and drift is what would
 * happen, quietly, with two copies of three putExtra calls.
 */
class OneLetterTest {

    private val mail = source("support/SupportMail.kt")
    private val screen = source("ui/VpnScreen.kt")

    @Test
    fun `both letters are filled by the same function`() {
        // Exactly one place sets the subject and the body. Two would be two
        // that can disagree.
        assertEquals(
            "the subject is set in more than one place, so the two letters " +
                "will drift apart",
            1,
            Regex("EXTRA_SUBJECT").findAll(mail).count(),
        )
        assertEquals(
            "the body is set in more than one place",
            1,
            Regex("EXTRA_TEXT").findAll(mail).count(),
        )
        assertTrue(
            "there is no shared filling, so each intent writes its own letter",
            mail.contains("private fun Intent.fill("),
        )
    }

    @Test
    fun `the recording goes to mail applications only`() {
        // ACTION_SEND is what carries an attachment; the selector is what puts
        // back the restriction ACTION_SENDTO gave for free. Without it this
        // offers every messenger on the phone a file listing the sites this
        // phone visited.
        assertTrue(
            "the recording letter has no mailto: selector, so it would be " +
                "offered to every application that can share a file",
            mail.contains("""selector = Intent(Intent.ACTION_SENDTO, Uri.parse("mailto:"))"""),
        )
    }

    @Test
    fun `the screen no longer opens a chooser for the recording`() {
        assertTrue(
            "the screen still builds a share chooser for the recording",
            !screen.contains("createChooser"),
        )
        assertTrue(
            "the screen does not send the recording through SupportMail",
            screen.contains("SupportMail.withRecording"),
        )
    }

    @Test
    fun `a recording is kept when nothing took it`() {
        // Deleting after a letter that never opened would destroy the one
        // thing the person spent five minutes producing.
        assertTrue(
            "the recording is dropped without checking that anything opened",
            screen.contains("if (opened) {"),
        )
    }

    private fun source(path: String): String {
        val file = java.io.File("src/main/kotlin/download/simplevpn/$path")
        assertTrue("cannot find $path", file.isFile)
        return file.readText()
    }
}
