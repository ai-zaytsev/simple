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
        // ACTION_SEND is what carries an attachment, and it is answered by
        // every messenger on the phone - which must not be offered a file
        // listing the sites this phone visited.
        //
        // The first attempt put a mailto: selector on the intent and left the
        // matching to the system. On a real phone nothing opened at all. So
        // the mail applications are now looked up by the intent that is known
        // to resolve here - the support button already uses it - and each is
        // addressed by name.
        assertTrue(
            "the mail applications are not looked up, so this relies on " +
                "intent matching that was already seen to fail",
            mail.contains("queryIntentActivities"),
        )
        assertTrue(
            "the letters are not addressed to a package, so any application " +
                "that can share a file could take one",
            mail.contains("setPackage("),
        )
    }

    @Test
    fun `a letter that does not open is reported`() {
        // The defect that mattered was not that it failed. It was that it
        // failed silently: the dialog closed and nothing else happened, which
        // is indistinguishable from the application being broken.
        assertTrue(
            "nothing tells the person when no letter opened",
            screen.contains("noMailApplication = true"),
        )
        assertTrue(
            "sending does not report whether anything opened",
            screen.contains("if (!sendRecordingByMail(context))"),
        )
    }

    @Test
    fun `the screen never builds a letter of its own`() {
        // The screen starts what SupportMail built and composes nothing. This
        // is what keeps the two letters identical: a second ACTION_SEND intent
        // here would be a second place with its own idea of the address, the
        // subject and the body.
        assertTrue(
            "the screen builds its own send intent, which is a second letter",
            !screen.contains("Intent(Intent.ACTION_SEND)"),
        )
        assertTrue(
            "the screen does not send the recording through SupportMail",
            screen.contains("SupportMail.withRecording"),
        )
    }

    @Test
    fun `a chooser is only ever offered the letters`() {
        // A chooser appears when the phone has more than one mail
        // application, and it must be built from the letters and nothing
        // else. The generic version of this - createChooser over a bare
        // ACTION_SEND - is what offered every messenger a file listing the
        // sites this phone visited.
        if (!screen.contains("createChooser")) return

        assertTrue(
            "a chooser is built without naming which intents it may offer",
            screen.contains("EXTRA_INITIAL_INTENTS"),
        )
        assertTrue(
            "the chooser is not built from the letters",
            screen.contains("createChooser(letters.first()"),
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
