package download.simplevpn.core

import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The name of the file decides whether anybody can be sent it.
 *
 * The share intent declared text/plain from the start, and that is not the
 * declaration that matters. A chooser reads it; the application the user picks
 * then resolves the content URI and asks the provider, and FileProvider
 * answers from Android's MIME table using the file name. There is no entry for
 * .log, so the answer was application/octet-stream - and a messenger refused
 * the attachment as unsupported while a mail client could not add it at all.
 * Both were correct, about a file that is plain text.
 *
 * This is asserted against the source rather than by exporting a file, because
 * exporting needs a Context and the fault is entirely in a constant. What it
 * guards is the pair: the extension the provider will read, and the type the
 * intent declares, staying the same kind of thing.
 */
class SharedFileNameTest {

    private val log = source("core/SessionLog.kt")
    private val screen = source("ui/VpnScreen.kt")

    @Test
    fun `the exported recording is a txt file`() {
        assertTrue(
            "the export is named something Android's MIME table does not know, " +
                "so the provider will call it application/octet-stream and " +
                "sending applications will refuse it",
            Regex("""TRACE_EXPORT_NAME = "[^"]+\.txt"""").containsMatchIn(log),
        )
    }

    @Test
    fun `the old name is still cleaned up`() {
        // An upgrade must not leave the previous export sitting in the cache.
        // It is unreachable from the screen and would be the file somebody
        // finds later and wonders about.
        assertTrue(
            "the previous export name is never deleted",
            log.contains("TRACE_EXPORT_WAS"),
        )
    }

    @Test
    fun `the recording is written as UTF-8`() {
        // It carries Russian text. This is already the default and a default
        // is a thing that can change - silently, into unreadable attachments.
        assertTrue(
            "the export does not name its encoding",
            log.contains("Charsets.UTF_8"),
        )
    }

    @Test
    fun `the share carries clip data as well as the extra`() {
        // The grant flag covers EXTRA_STREAM in most receivers and not all.
        // A receiver that reads the URI off the clip data gets an empty file
        // without this - the worst failure of the three, because it looks like
        // it worked.
        assertTrue(
            "the share intent has no clip data, so some receivers get no grant",
            screen.contains("ClipData.newUri"),
        )
        assertTrue(
            "the share intent no longer declares a type",
            screen.contains("""type = "text/plain""""),
        )
    }

    private fun source(path: String): String {
        val file = java.io.File("src/main/kotlin/download/simplevpn/$path")
        assertTrue("cannot find $path", file.isFile)
        return file.readText()
    }
}
