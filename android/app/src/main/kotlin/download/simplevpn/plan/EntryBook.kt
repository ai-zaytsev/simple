package download.simplevpn.plan

import android.content.Context
import android.util.Log
import org.json.JSONArray
import org.json.JSONObject

/**
 * Every way this installation knows of reaching the Control Plane.
 *
 * The list starts as what the build was born with and becomes whatever the
 * signed descriptor last said. That replacement is the whole point: an entry
 * that gets blocked is replaced by an operator in minutes, where a new build
 * takes days to reach anybody and never reaches everybody.
 *
 * The descriptor is checked the same way every other document is - signature
 * first, then a sequence number that must be higher than the last one applied.
 * The sequence rule matters more here than anywhere else: without it an
 * adversary who recorded an old descriptor could serve it back and walk every
 * client onto the entries they have already blocked.
 *
 * A descriptor that cannot be fetched changes nothing. The stored list stays,
 * whatever its age, because the moment this list is needed is precisely the
 * moment the service is hard to reach.
 */
class EntryBook(context: Context) {

    private val prefs = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    /** What to try, newest known list first, falling back to the build's own. */
    fun entries(): List<Entry> {
        val stored = read()
        return if (stored.isEmpty()) Entry.SEED else stored
    }

    val lastSeq: Long get() = prefs.getLong(KEY_SEQ, 0L)

    /**
     * Takes a descriptor that has already had its signature checked.
     *
     * @return true when it replaced what was stored
     */
    fun accept(payload: JSONObject): Boolean {
        val seq = payload.optLong("seq", 0)
        if (seq <= lastSeq) {
            Log.i(TAG, "descriptor is not newer than the one in use")
            return false
        }

        val array = payload.optJSONArray("entries") ?: return false
        val entries = buildList {
            for (i in 0 until array.length()) {
                val entry = array.optJSONObject(i) ?: continue
                Entry.parse(entry)?.let(::add)
            }
        }

        // A descriptor naming nothing this build can use is refused rather than
        // stored. Storing it would leave an installation with an empty list and
        // no way to be told a better one - the failure this whole mechanism
        // exists to prevent, arriving through the mechanism itself.
        if (entries.isEmpty()) {
            Log.w(TAG, "descriptor names no entry this build understands")
            return false
        }

        prefs.edit()
            .putString(KEY_ENTRIES, array.toString())
            .putLong(KEY_SEQ, seq)
            .apply()
        Log.i(TAG, "descriptor accepted: ${entries.size} entries, seq $seq")
        return true
    }

    private fun read(): List<Entry> {
        val raw = prefs.getString(KEY_ENTRIES, null) ?: return emptyList()
        return try {
            val array = JSONArray(raw)
            buildList {
                for (i in 0 until array.length()) {
                    val entry = array.optJSONObject(i) ?: continue
                    Entry.parse(entry)?.let(::add)
                }
            }
        } catch (t: Throwable) {
            Log.w(TAG, "stored descriptor is unreadable", t)
            emptyList()
        }
    }

    private companion object {
        const val TAG = "EntryBook"
        const val NAME = "entries"
        const val KEY_ENTRIES = "entries"
        const val KEY_SEQ = "seq"
    }
}
