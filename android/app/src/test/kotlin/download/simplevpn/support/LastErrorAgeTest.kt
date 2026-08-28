package download.simplevpn.support

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * How long ago a failure was, in words rather than minutes.
 *
 * Coarse on purpose, and the coarseness is the design. "17 минут назад" is no
 * more useful to support than "меньше часа назад", and the precise figure says
 * more about when this person was using the service than about the fault. The
 * letter is written by us and sent by them; it should not carry a timestamp
 * they did not think to give.
 */
class LastErrorAgeTest {

    private val minute = 60_000L
    private val hour = 60 * minute
    private val day = 24 * hour

    private fun ago(since: Long) = LastError.ago(atMillis = 0L, nowMillis = since)

    @Test
    fun `recent failures are within the hour`() {
        assertEquals("меньше часа назад", ago(0))
        assertEquals("меньше часа назад", ago(17 * minute))
        assertEquals("меньше часа назад", ago(59 * minute))
    }

    @Test
    fun `an hour is already today rather than a number`() {
        assertEquals("сегодня", ago(hour))
        assertEquals("сегодня", ago(23 * hour))
    }

    @Test
    fun `a day is this week`() {
        assertEquals("на этой неделе", ago(day))
        assertEquals("на этой неделе", ago(6 * day))
    }

    @Test
    fun `older than a week stops being a time at all`() {
        assertEquals("давно", ago(7 * day))
        assertEquals("давно", ago(400 * day))
    }

    /**
     * A phone whose clock moved backwards - a timezone change, a manual
     * correction, a reboot - would otherwise produce a negative age and a
     * letter saying the failure happens in the future.
     */
    @Test
    fun `a clock that moved backwards does not produce a time from the future`() {
        assertEquals("недавно", LastError.ago(atMillis = hour, nowMillis = 0L))
    }
}
