package download.simplevpn.ui

import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * The two things this application can send, and why they must not read alike.
 *
 * One is a report of what the application did: endpoints, transports,
 * outcomes, no addresses. It is safe to send to anybody. The other lists the
 * sites the phone connected to while a recording ran.
 *
 * They were once labelled "Отправить лог" and "Отправить логи", one letter
 * apart, sitting on the same screen. Nobody could be expected to tell them
 * apart, and telling them apart is the entire point: the cost of sending the
 * wrong one falls on the person who owns the phone.
 *
 * The screen itself has no test - this project has no Compose test harness,
 * and adding one for this would be a bigger change than the fix. What can be
 * checked without one is the wording, which is where the confusion lived.
 */
class LogActionsTest {

    private val strings: String by lazy {
        val file = File("src/main/res/values/strings.xml")
        assertTrue("cannot find ${file.absolutePath}", file.isFile)
        file.readText()
    }

    private fun value(name: String): String {
        val found = Regex("""<string name="$name">(.*?)</string>""", RegexOption.DOT_MATCHES_ALL)
            .find(strings)
        assertNotNull("$name is missing from strings.xml", found)
        return found!!.groupValues[1].trim()
    }

    @Test
    fun `the two things that can be sent do not read alike`() {
        val ordinary = value("action_share_log")
        val recording = value("trace_send")

        assertTrue(
            "both actions are labelled \"$ordinary\"",
            !ordinary.equals(recording, ignoreCase = true),
        )

        // Differing only by an ending is not differing. "Отправить лог" and
        // "Отправить логи" are two different files and one label.
        val shorter = minOf(ordinary, recording, compareBy { it.length })
        val longer = maxOf(ordinary, recording, compareBy { it.length })
        assertTrue(
            "\"$ordinary\" and \"$recording\" differ only by an ending; " +
                "on the same screen they are one label",
            !longer.startsWith(shorter, ignoreCase = true),
        )
    }

    @Test
    fun `the recording is the one that warns, and says so in its own words`() {
        // The warning has to name what it is about. A dialog that says "this
        // may contain diagnostic information" warns nobody about anything.
        val warning = value("trace_warning_body")
        assertTrue(
            "the warning before a recording does not say what it records",
            listOf("сайт", "сервис").any { warning.contains(it, ignoreCase = true) },
        )
    }
}
