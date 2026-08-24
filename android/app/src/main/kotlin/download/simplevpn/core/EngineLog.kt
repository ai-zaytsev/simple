package download.simplevpn.core

import android.content.Context
import android.util.Log
import java.io.File
import java.io.RandomAccessFile

/**
 * The engine's own account of what went wrong, brought to the screen.
 *
 * Everything else in this build reports what a check observed from outside.
 * This reports what the engine itself says, which is the difference between
 * "the connection timed out" and the reason it did.
 *
 * Diagnostic only. The engine writes this at a level that names destinations,
 * so the file stays in private storage, is truncated at every start, and goes
 * away with the slice together with the level that fills it.
 */
object EngineLog {

    fun file(context: Context): File = File(context.filesDir, NAME)

    /** Truncates before a run, so what is shown belongs to this session. */
    fun reset(context: Context) {
        runCatching { file(context).delete() }
            .onFailure { Log.w(TAG, "could not clear the engine log", it) }
    }

    /**
     * The most recent line describing a failure, or the last line if none does.
     * Only the tail is read: the file grows for as long as the tunnel runs.
     */
    fun lastFailure(context: Context): String {
        val target = file(context)
        if (!target.isFile || target.length() == 0L) return "log: engine wrote nothing"

        return try {
            val tail = RandomAccessFile(target, "r").use { handle ->
                val from = maxOf(0L, handle.length() - TAIL_BYTES)
                handle.seek(from)
                val buffer = ByteArray((handle.length() - from).toInt())
                handle.readFully(buffer)
                String(buffer)
            }

            val lines = tail.lineSequence().filter { it.isNotBlank() }.toList()

            // Errors first, and only then anything merely mentioning a failure.
            // Background telemetry names fail to resolve on any device, all day,
            // at info level. Showing the newest of those buries the one line
            // that would say something is actually wrong.
            val chosen = lines.lastOrNull { it.contains(ERROR_LEVEL) }
                ?: lines.lastOrNull { line -> MARKERS.any { line.contains(it, ignoreCase = true) } }
                ?: lines.lastOrNull()
                ?: return "log: engine wrote nothing"
            "log: " + chosen.takeLast(MAX_LENGTH).trim()
        } catch (t: Throwable) {
            Log.w(TAG, "could not read the engine log", t)
            "log: unreadable"
        }
    }

    private const val TAG = "EngineLog"
    private const val NAME = "engine.log"
    private const val TAIL_BYTES = 16_384L
    private const val MAX_LENGTH = 160
    private const val ERROR_LEVEL = "[Error]"
    private val MARKERS = listOf("failed", "rejected", "refused", "timeout", "invalid")
}
