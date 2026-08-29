package download.simplevpn.core

import android.content.Context
import android.util.Log
import java.io.File
import java.io.RandomAccessFile
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * What a session produced, kept as three files with three different rules.
 *
 * The split is the whole design, because the three differ in what they can
 * reveal about the person holding the phone:
 *
 * [appFile] is the application's own account - which endpoint, which
 * transport, what happened. It names no destination, so it is always kept and
 * always safe to send. Nearly every fault found in this product was found
 * here.
 *
 * [engineFile] is the engine at `warning`. Also kept always, also safe:
 * verified on a live node that at this level Xray writes nothing about where
 * it connected, even when a dial fails.
 *
 * [traceFile] is the engine at `info`, which names every address it dials. It
 * is a browsing history, and it exists only while somebody has deliberately
 * started a recording, having been told what it holds. It is removed when it
 * is sent and when the application starts, so that at an arbitrary moment
 * there is nothing on the device to take.
 *
 * Separate files rather than one, for a second reason too: the engine holds
 * its own handle and writes directly, so interleaving our lines into it would
 * tear them exactly when the file matters.
 */
object SessionLog {

    /** The application's own account of the session. Never names a destination. */
    fun appFile(context: Context): File = File(context.filesDir, APP_NAME)

    /** The engine's faults. Never names a destination. */
    fun engineFile(context: Context): File = File(context.filesDir, ENGINE_NAME)

    /**
     * The detailed recording, which does name destinations.
     *
     * Deliberately not merged into [engineFile]: if it were, an ordinary
     * support log sent a week later would still be carrying the history of
     * whatever was recorded once.
     */
    fun traceFile(context: Context): File = File(context.filesDir, TRACE_NAME)

    /** Starts a session. Both ordinary accounts are cleared so the file is one attempt. */
    fun reset(context: Context) {
        runCatching { appFile(context).delete() }
        runCatching { engineFile(context).delete() }
        record(context, "session start")
    }

    /**
     * Removes the detailed recording.
     *
     * Called when it has been sent and when the application starts. Not on
     * every session start: a recording is meant to survive the reconnect that
     * starting it causes, and a user who records a fault, sees it, and then
     * reconnects should still have the file.
     */
    fun dropTrace(context: Context) {
        runCatching { traceFile(context).delete() }
        runCatching { File(context.cacheDir, TRACE_EXPORT_NAME).delete() }
        runCatching { File(context.cacheDir, TRACE_EXPORT_WAS).delete() }
    }

    /** Whether a recording is sitting on the device waiting to be sent. */
    fun hasTrace(context: Context): Boolean =
        runCatching { traceFile(context).length() > 0 }.getOrDefault(false)

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
     * Packs the detailed recording for sending, saying plainly what is in it.
     *
     * The header is not decoration. Somebody about to attach this to a message
     * has usually never looked inside one, and the last person to send one had
     * no idea it listed seventy-four sites they had visited in under four
     * minutes.
     */
    fun exportTrace(context: Context): File? {
        return try {
            val trace = traceFile(context)
            if (!trace.isFile || trace.length() == 0L) return null

            val target = File(context.cacheDir, TRACE_EXPORT_NAME)
            target.parentFile?.mkdirs()

            val text = buildString {
                appendLine("# Simple VPN detailed recording")
                appendLine("#")
                appendLine("# THIS FILE LISTS THE SITES AND SERVICES THIS DEVICE CONNECTED TO")
                appendLine("# while the recording was running. Send it only to someone you")
                appendLine("# mean to show that to.")
                appendLine()
                appendLine("## What the application did")
                appendLine()
                append(readAll(appFile(context)).ifBlank { "(nothing recorded)\n" })
                appendLine()
                appendLine("## What the engine saw")
                appendLine()
                append(readTail(trace).ifBlank { "(the recording is empty)\n" })
            }

            // UTF-8 said out loud. It is already Kotlin's default and the file
            // carries Russian text; a default is a thing that can change, and
            // this one would change silently into unreadable attachments.
            target.writeText(text, Charsets.UTF_8)
            target
        } catch (t: Throwable) {
            Log.e(TAG, "could not export the recording", t)
            null
        }
    }

    /**
     * How many distinct destinations the recording holds.
     *
     * Shown to the user before they send it, because a number makes the
     * abstract concrete: "74 sites" is understood immediately and "may contain
     * information about services you used" is not.
     */
    fun traceDestinations(context: Context): Int = try {
        val trace = traceFile(context)
        if (!trace.isFile) {
            0
        } else {
            SNIFFED.findAll(readTail(trace))
                .map { it.groupValues[1] }
                .toHashSet()
                .size
        }
    } catch (t: Throwable) {
        Log.w(TAG, "could not count what the recording holds", t)
        0
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
    private const val TRACE_NAME = "trace.log"
    // .txt, and the extension is the whole of it.
    //
    // The share intent already declared text/plain and that is not what the
    // receiving application reads. It resolves the content URI and asks the
    // provider, and FileProvider derives the type from the file name against
    // Android's MIME table - which has no entry for .log, so the answer was
    // application/octet-stream. A messenger refused it as an unsupported
    // attachment and a mail client could not add it at all, both correctly
    // and both about a file that is plain text.
    //
    // The old name is still removed on cleanup below, so an upgrade does not
    // leave one behind in the cache.
    private const val TRACE_EXPORT_NAME = "logs/simple-vpn-recording.txt"
    private const val TRACE_EXPORT_WAS = "logs/simple-vpn-recording.log"

    /** How the engine names a destination it has recognised. */
    private val SNIFFED = Regex("sniffed domain: ([a-zA-Z0-9._-]+)")
    private const val TIME_FORMAT = "HH:mm:ss.SSS"
    private const val TAIL_BYTES = 512_000L
}
