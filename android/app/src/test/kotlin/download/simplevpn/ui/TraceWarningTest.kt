package download.simplevpn.ui

import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * What the user is told before a recording starts.
 *
 * The warning is the whole reason the recording is allowed to exist. Without
 * it this is the old defect with a button attached: a phone keeping a list of
 * the sites its owner visited, which somebody then sends on without knowing
 * what is in it. That is not hypothetical - it is how the last one was sent.
 *
 * So the warning has to name what it is about. "May contain diagnostic
 * information" warns nobody about anything.
 */
class TraceWarningTest {

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
    fun `the warning says what the recording records`() {
        val warning = value("trace_warning_body")
        assertTrue(
            "the warning before a recording does not say what it records",
            listOf("сайт", "сервис").any { warning.contains(it, ignoreCase = true) },
        )
    }

    @Test
    fun `the warning says the recording stops on its own`() {
        // Somebody who believes they must remember to stop it will worry about
        // it; somebody told it stops itself will use it. Both are reasons to
        // say so, and it is also simply true.
        val warning = value("trace_warning_body")
        assertTrue(
            "the warning does not mention that the recording ends by itself",
            warning.contains("10 минут") || warning.contains("десять минут"),
        )
    }

    @Test
    fun `sending says how much is in the file`() {
        // A number, not an adjective. "74 адресов" is understood immediately;
        // "may contain information about services you used" is understood by
        // nobody, which is exactly how the last recording left a device.
        val body = value("trace_send_body")
        assertTrue(
            "the send dialog does not put a number in front of the person sending",
            body.contains("%1\$d"),
        )
    }
}
