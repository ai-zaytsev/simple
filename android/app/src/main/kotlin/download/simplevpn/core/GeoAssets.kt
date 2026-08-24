package download.simplevpn.core

import android.content.Context
import android.util.Log
import java.io.File

/**
 * Puts the engine's geographic database where the engine can read it.
 *
 * The routing rules send Russian addresses and private ranges straight out,
 * and both are expressed as geoip entries. The engine reads those from a file
 * on disk, not from the package, and refuses the whole configuration when the
 * file is absent. So this is not an optimisation: without it the engine never
 * starts and the tunnel never comes up.
 *
 * The copy is repeated after an application update, because a new build may
 * ship a newer database. It is skipped otherwise, since the file is large
 * enough that copying it on every connection would be felt.
 */
object GeoAssets {

    /** Returns the directory the engine should be pointed at, or null. */
    fun install(context: Context): File? {
        return try {
            val dir = File(context.filesDir, DIRECTORY)
            if (!dir.isDirectory && !dir.mkdirs()) {
                Log.e(TAG, "could not create the asset directory")
                return null
            }

            val database = File(dir, DATABASE)
            val stamp = File(dir, STAMP)
            val current = installedAt(context).toString()

            if (database.isFile && stamp.isFile && stamp.readText() == current) {
                return dir
            }

            context.assets.open(DATABASE).use { source ->
                database.outputStream().use { target -> source.copyTo(target) }
            }
            stamp.writeText(current)

            Log.i(TAG, "installed the geographic database, ${database.length()} bytes")
            dir
        } catch (t: Throwable) {
            // Includes the case where the database was never bundled. Returning
            // null lets the caller report a build problem rather than let the
            // engine fail on a configuration the user cannot influence.
            Log.e(TAG, "could not install the geographic database", t)
            null
        }
    }

    /**
     * Identifies the installed build. Version code is not enough: two debug
     * builds of different commits share it, and the database would then never
     * be refreshed on a test device.
     */
    private fun installedAt(context: Context): Long = try {
        context.packageManager.getPackageInfo(context.packageName, 0).lastUpdateTime
    } catch (t: Throwable) {
        Log.w(TAG, "could not read the install time", t)
        0L
    }

    private const val TAG = "GeoAssets"
    private const val DIRECTORY = "xray"
    private const val DATABASE = "geoip.dat"
    private const val STAMP = "geoip.stamp"
}
