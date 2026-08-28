package download.simplevpn.support

import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * What may and may not travel in a support message.
 *
 * The letter goes through the person's own mail account, their provider and
 * ours. Anything put into it automatically is put there by us on somebody's
 * behalf, so the interesting tests here are not that the facts arrive - they
 * are that nothing else does.
 */
class SupportRequestTest {

    private val installation = "0b1f4c2e-9a77-4d61-b0e3-3a6c1f2d8e94"

    private fun facts(lastError: String? = null) = SupportRequest.Facts(
        appVersion = "0.1.0",
        deviceModel = "Xiaomi Redmi Note 12",
        androidVersion = "13 (API 33)",
        deviceId = installation,
        lastError = lastError,
        lastErrorAgo = lastError?.let { "меньше часа назад" },
    )

    @Test
    fun `the letter carries every fact support needs`() {
        val body = SupportRequest.body(facts("Не удалось создать сетевой интерфейс"))

        listOf(
            "0.1.0",
            "Xiaomi Redmi Note 12",
            "13 (API 33)",
            installation,
            "Не удалось создать сетевой интерфейс",
            "меньше часа назад",
        ).forEach {
            assertTrue("the letter does not mention $it:\n$body", body.contains(it))
        }
    }

    @Test
    fun `the letter opens by asking, and leaves room for the answer`() {
        val body = SupportRequest.body(facts())

        // Somebody typing under eight lines of diagnostics writes a footnote,
        // and somebody opening a blank message writes less than somebody
        // answering a question.
        assertTrue(
            "the letter does not open by asking what is wrong:\n$body",
            body.startsWith(SupportRequest.WRITE_HERE),
        )

        val beforeFacts = body.substringBefore("— — —")
        assertTrue(
            "there is nowhere to write between the question and the facts:\n$body",
            beforeFacts.lines().count { it.isBlank() } >= 2,
        )
    }

    @Test
    fun `no error means no line about one`() {
        val body = SupportRequest.body(facts(lastError = null))
        assertFalse(
            "a letter with no error to report mentions one:\n$body",
            body.contains("Последняя ошибка"),
        )
    }

    @Test
    fun `an empty error is the same as no error`() {
        val body = SupportRequest.body(facts(lastError = "   "))
        assertFalse(body.contains("Последняя ошибка"))
    }

    /**
     * The one field that is not a fixed fact is the error message, and a
     * message written a year from now could carry a token into it without
     * anybody meaning to. This is the guard behind the shape of Facts.
     */
    @Test
    fun `anything shaped like a key does not travel`() {
        val secrets = listOf(
            // A base64 key of the kind the transport uses.
            "yLXQrM2FnFxvBQ0oJhVZK7cRt4Wm8sPdA1uEbNq3iYc=",
            // A hex token.
            "9f3c1a7b2e4d6081a5c9e2f47b3d6018a9c4e7f21b5d8036",
            // A long opaque identifier.
            "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9abcdefghijkl",
        )

        secrets.forEach { secret ->
            val body = SupportRequest.body(facts("Не удалось подключиться: $secret"))
            assertFalse(
                "a secret survived into the letter:\n$body",
                body.contains(secret),
            )
        }
    }

    @Test
    fun `a value announced as a secret does not travel`() {
        listOf(
            "device token abc123",
            "key=hunter2",
            "пароль: qwerty",
            "токен zzz9",
        ).forEach { phrase ->
            val body = SupportRequest.body(facts("Ошибка, $phrase"))
            val leaked = phrase.substringAfterLast(' ').substringAfterLast('=')
                .substringAfterLast(':')
            assertFalse(
                "\"$phrase\" left its value in the letter:\n$body",
                body.contains(leaked),
            )
        }
    }

    /**
     * The installation identifier is the one long value that must survive. It
     * is what lets us find this device's own reports; it is not a credential,
     * and the device authenticates with something else entirely. A guard that
     * hid it would make the letter useless while looking careful.
     */
    @Test
    fun `the identifier of the installation survives`() {
        val body = SupportRequest.body(facts())
        assertTrue(
            "the installation identifier was scrubbed out of the letter:\n$body",
            body.contains(installation),
        )
    }

    @Test
    fun `ordinary words are left alone`() {
        val message = "Не удалось подключиться ни с текущими, ни с предыдущими настройками"
        val body = SupportRequest.body(facts(message))
        assertTrue("an ordinary message was mangled:\n$body", body.contains(message))
    }

    @Test
    fun `a multi-line error becomes one line`() {
        val body = SupportRequest.body(facts("Первая строка\nвторая строка"))
        assertTrue(body.contains("Первая строка вторая строка"))
    }

    @Test
    fun `the subject says which version is writing`() {
        assertTrue(SupportRequest.subject("0.1.0").contains("0.1.0"))
    }

    /**
     * The address is a fact about the product, so it is checked here rather
     * than trusted: a support button pointing at an address that does not
     * exist is worse than no button, because the person believes they have
     * written to somebody.
     */
    @Test
    fun `the support address is an address`() {
        val strings = File("src/main/res/values/strings.xml")
        assertTrue("cannot find ${strings.absolutePath}", strings.isFile)

        val found = Regex("""<string name="support_email">(.*?)</string>""")
            .find(strings.readText())
        assertNotNull("support_email is missing from strings.xml", found)

        val address = found!!.groupValues[1].trim()
        assertTrue("$address is not an address", address.matches(Regex("""[^@\s]+@[^@\s]+\.[a-z]+""")))
    }
}
