package download.simplevpn.support

import android.content.Context

/**
 * The last thing that went wrong, kept so a support message can mention it.
 *
 * Kept on disk rather than in memory because of when it is read: somebody whose
 * connection failed closes the application, tries a few things, opens it again
 * and only then writes to us. An error held only in a flow is gone by then, and
 * "у меня не работает" with no reason is the message we would otherwise get.
 *
 * One value, overwritten. A history of failures would be a record of when this
 * person used the service, which is exactly the kind of thing this system does
 * not keep.
 */
object LastError {

    fun record(context: Context, reason: String) {
        prefs(context).edit()
            .putString(KEY_REASON, reason)
            .putLong(KEY_AT, System.currentTimeMillis())
            .apply()
    }

    fun read(context: Context): Recorded? {
        val stored = prefs(context).getString(KEY_REASON, null) ?: return null
        val at = prefs(context).getLong(KEY_AT, 0L)
        return Recorded(reason = stored, atMillis = at)
    }

    data class Recorded(val reason: String, val atMillis: Long)

    /**
     * How long ago it was, in words a person would use.
     *
     * Coarse on purpose. A support message saying "17 минут назад" is no more
     * useful than one saying "меньше часа назад", and the precise figure says
     * more about when somebody was using the service than about the fault.
     */
    fun ago(atMillis: Long, nowMillis: Long): String {
        val minutes = (nowMillis - atMillis) / 60_000
        return when {
            minutes < 0L -> "недавно"
            minutes < 60L -> "меньше часа назад"
            minutes < 24L * 60L -> "сегодня"
            minutes < 7L * 24L * 60L -> "на этой неделе"
            else -> "давно"
        }
    }

    private fun prefs(context: Context) =
        context.getSharedPreferences("support", Context.MODE_PRIVATE)

    private const val KEY_REASON = "last_error"
    private const val KEY_AT = "last_error_at"
}
