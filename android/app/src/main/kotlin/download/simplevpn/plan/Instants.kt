package download.simplevpn.plan

import java.text.SimpleDateFormat
import java.util.Locale
import java.util.TimeZone

/**
 * Parses the timestamps the Control Plane issues.
 *
 * Written by hand rather than with the modern date API because that arrived at
 * API level 26 and the minimum supported level is 24. The format is fixed by
 * the server, so a parser that accepts exactly that format and nothing else is
 * the honest amount of flexibility.
 */
internal object Instants {

    fun parse(value: String): Long? = try {
        format().parse(value)?.time
    } catch (t: Throwable) {
        null
    }

    private fun format() = SimpleDateFormat(PATTERN, Locale.US).apply {
        timeZone = TimeZone.getTimeZone("UTC")
        isLenient = false
    }

    private const val PATTERN = "yyyy-MM-dd'T'HH:mm:ss'Z'"
}
