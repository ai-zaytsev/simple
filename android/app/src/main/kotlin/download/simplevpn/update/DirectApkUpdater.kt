package download.simplevpn.update

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.os.Build
import java.io.File
import java.net.URI
import java.security.MessageDigest
import javax.net.ssl.HttpsURLConnection

/** Downloads one signed-policy APK and commits it to Android's installer. */
internal object DirectApkUpdater {

    sealed interface Result {
        data object Committed : Result
        data class Failed(val failure: UpdateFailure) : Result
    }

    fun mayInstall(context: Context): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.O ||
            context.packageManager.canRequestPackageInstalls()

    fun install(
        context: Context,
        artifact: AppUpdatePolicy.Artifact,
        versionCode: Int,
    ): Result {
        if (!artifact.isValidDirectApk()) return Result.Failed(UpdateFailure.UNAVAILABLE)
        val apk = try {
            download(context, artifact, versionCode)
        } catch (_: HashMismatch) {
            return Result.Failed(UpdateFailure.HASH_MISMATCH)
        } catch (_: Throwable) {
            return Result.Failed(UpdateFailure.DOWNLOAD)
        }

        return try {
            commit(context, apk)
            Result.Committed
        } catch (_: Throwable) {
            apk.delete()
            Result.Failed(UpdateFailure.INSTALLER)
        }
    }

    private fun download(
        context: Context,
        artifact: AppUpdatePolicy.Artifact,
        versionCode: Int,
    ): File {
        val directory = File(context.cacheDir, "updates").apply { mkdirs() }
        directory.listFiles()?.forEach { it.delete() }
        val partial = File(directory, "simple-vpn-$versionCode.apk.part")
        val ready = File(directory, "simple-vpn-$versionCode.apk")

        try {
            var address = URI(artifact.url)
            repeat(MAX_REDIRECTS + 1) { redirect ->
                val opened = address.toURL().openConnection()
                val connection = opened as? HttpsURLConnection
                    ?: throw IllegalArgumentException("update is not HTTPS")
                connection.instanceFollowRedirects = false
                connection.connectTimeout = CONNECT_TIMEOUT_MS
                connection.readTimeout = READ_TIMEOUT_MS
                connection.setRequestProperty("Accept", APK_MIME)
                try {
                    when (val code = connection.responseCode) {
                        in 300..399 -> {
                            if (redirect == MAX_REDIRECTS) throw IllegalStateException("too many redirects")
                            val next = connection.getHeaderField("Location")
                                ?: throw IllegalStateException("redirect has no location")
                            address = address.resolve(next)
                            if (address.scheme != "https" || address.host.isNullOrBlank()) {
                                throw IllegalStateException("redirect left HTTPS")
                            }
                        }

                        HttpsURLConnection.HTTP_OK -> {
                            val declared = connection.contentLengthLong
                            if (declared > MAX_APK_BYTES) {
                                throw IllegalStateException("APK size is not acceptable")
                            }
                            val digest = MessageDigest.getInstance("SHA-256")
                            var received = 0L
                            connection.inputStream.use { input ->
                                partial.outputStream().use { output ->
                                    val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
                                    while (true) {
                                        val count = input.read(buffer)
                                        if (count < 0) break
                                        received += count
                                        if (received > MAX_APK_BYTES) {
                                            throw IllegalStateException("APK exceeds size limit")
                                        }
                                        digest.update(buffer, 0, count)
                                        output.write(buffer, 0, count)
                                    }
                                }
                            }
                            if (received <= 0 || (declared >= 0 && received != declared)) {
                                throw IllegalStateException("APK download is incomplete")
                            }
                            if (!sha256Matches(digest.digest(), artifact.sha256)) throw HashMismatch()
                            if (!partial.renameTo(ready)) throw IllegalStateException("cannot finish APK file")
                            return ready
                        }

                        else -> throw IllegalStateException("APK returned HTTP $code")
                    }
                } finally {
                    connection.disconnect()
                }
            }
            throw IllegalStateException("APK redirect loop")
        } catch (problem: Throwable) {
            partial.delete()
            ready.delete()
            throw problem
        }
    }

    private fun commit(context: Context, apk: File) {
        val installer = context.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
            setAppPackageName(context.packageName)
            setSize(apk.length())
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                setInstallReason(PackageManager.INSTALL_REASON_USER)
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_REQUIRED)
            }
        }
        val sessionId = installer.createSession(params)
        var committed = false
        try {
            installer.openSession(sessionId).use { session ->
                apk.inputStream().use { input ->
                    session.openWrite("base.apk", 0, apk.length()).use { output ->
                        input.copyTo(output)
                        session.fsync(output)
                    }
                }
                val callback = Intent(context, UpdateInstallReceiver::class.java).apply {
                    action = "${context.packageName}.UPDATE_INSTALL_RESULT"
                }
                val flags = PendingIntent.FLAG_UPDATE_CURRENT or
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) PendingIntent.FLAG_MUTABLE else 0
                val pending = PendingIntent.getBroadcast(context, sessionId, callback, flags)
                session.commit(pending.intentSender)
                committed = true
            }
        } finally {
            if (!committed) installer.abandonSession(sessionId)
        }
    }

    private class HashMismatch : Exception()

    private const val APK_MIME = "application/vnd.android.package-archive"
    private const val CONNECT_TIMEOUT_MS = 15_000
    private const val READ_TIMEOUT_MS = 180_000
    private const val MAX_APK_BYTES = 200L * 1024 * 1024
    private const val MAX_REDIRECTS = 3
}

internal fun sha256Matches(actualDigest: ByteArray, expectedHex: String): Boolean =
    actualDigest.joinToString("") { "%02x".format(it) } == expectedHex

enum class UpdateFailure { UNAVAILABLE, DOWNLOAD, HASH_MISMATCH, PERMISSION, INSTALLER }
