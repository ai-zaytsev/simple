package download.simplevpn.core

import android.content.Context
import android.util.Log
import java.io.File
import java.io.RandomAccessFile
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * Everything one connection attempt produced, in one file the user can send.
 *
 * Reading numbers off a screen answered the question of which layer was broken
 * but never why, and each round of guessing cost an install. This keeps the
 * whole account of a session instead: what the application did, in order, and
 * what the engine said about it, from the moment ON is pressed until the tunnel
 * is stopped.
 *
 * Two files rather than one, because the engine holds its own open and writes
 * to it directly; interleaving our own writes into the same handle would
 * produce torn lines exactly when the file matters most. They are merged when
 * the log is exported.
 *
 * Diagnostic only, and deliberately short-lived. The engine writes at a level
 * that names every destination it dials, which is the browsing history the
 * privacy model forbids keeping, so the file lives in private storage, is
 * truncated at the start of every session, and leaves the device only when the
 * user explicitly shares it. This whole class goes when the slice does.
 */
object SessionLog {

    /** The application's own account of the session. */
    fun appFile(context: Context): File = File(context.filesDir, APP_NAME)

    /** The engine's account, written by the engine itself. */
    fun engineFile(context: Context): File = File(context.filesDir, ENGINE_NAME)

    /** Starts a session. Both accounts are cleared so the file is one attempt. */
    fun reset(context: Context) {
        runCatching { appFile(context).delete() }
        runCatching { engineFile(context).delete() }
        record(context, "session start")
    }

    /** Appends one timestamped line to the application's account. */
    fun record(context: Context, line: String) {
        try {
            val stamp = SimpleDateFormat(TIME_FORMAT, Locale.US).format(Date())
            appFile(context).appendText("$stamp  $line\n")
        } catch (t: Throwable) {
            Log.w(TAG, "could not record: $line", t)
        }
    }

    /**
     * Merges both accounts into one file for sharing.
     *
     * The application's account comes first and whole: it is short, ordered,
     * and says what was attempted. The engine's account follows, trimmed to its
     * tail, because it grows for as long as the tunnel runs and the end is
     * where a failure is.
     */
    fun export(context: Context): File? {
        return try {
            val target = File(context.cacheDir, EXPORT_NAME)
            target.parentFile?.mkdirs()

            val text = buildString {
                appendLine("# Simple VPN session log")
                appendLine("# Diagnostic build. Contains the addresses this device connected to.")
                appendLine()
                appendLine("## What the application did")
                appendLine()
                append(readAll(appFile(context)).ifBlank { "(nothing recorded)\n" })
                appendLine()
                appendLine("## What the engine said")
                appendLine()
                append(readTail(engineFile(context)).ifBlank { "(engine wrote nothing)\n" })
            }

            target.writeText(text)
            target
        } catch (t: Throwable) {
            Log.e(TAG, "could not export the session log", t)
            null
        }
    }

    /** The line worth showing on screen, preferring an actual error. */
    fun lastFailure(context: Context): String {
        val tail = readTail(engineFile(context))
        if (tail.isBlank()) return "log: engine wrote nothing"

        val lines = tail.lineSequence().filter { it.isNotBlank() }.toList()

        // Background telemetry names fail to resolve on any device, all day, at
        // info level. Showing the newest of those buries anything real.
        val chosen = lines.lastOrNull { it.contains(ERROR_LEVEL) }
            ?: lines.lastOrNull { line -> MARKERS.any { line.contains(it, ignoreCase = true) } }
            ?: lines.lastOrNull()
            ?: return "log: engine wrote nothing"

        return "log: " + chosen.takeLast(MAX_LENGTH).trim()
    }

    private fun readAll(file: File): String = try {
        if (file.isFile) file.readText() else ""
    } catch (t: Throwable) {
        Log.w(TAG, "could not read ${file.name}", t)
        ""
    }

    private fun readTail(file: File): String = try {
        if (!file.isFile || file.length() == 0L) {
            ""
        } else {
            RandomAccessFile(file, "r").use { handle ->
                val from = maxOf(0L, handle.length() - TAIL_BYTES)
                handle.seek(from)
                val buffer = ByteArray((handle.length() - from).toInt())
                handle.readFully(buffer)
                String(buffer)
            }
        }
    } catch (t: Throwable) {
        Log.w(TAG, "could not read ${file.name}", t)
        ""
    }

    private const val TAG = "SessionLog"
    private const val APP_NAME = "session.log"
    private const val ENGINE_NAME = "engine.log"
    private const val EXPORT_NAME = "logs/simple-vpn-session.log"
    private const val TIME_FORMAT = "HH:mm:ss.SSS"
    private const val TAIL_BYTES = 512_000L
    private const val MAX_LENGTH = 160
    private const val ERROR_LEVEL = "[Error]"
    private val MARKERS = listOf("failed", "rejected", "refused", "timeout", "invalid")
}
