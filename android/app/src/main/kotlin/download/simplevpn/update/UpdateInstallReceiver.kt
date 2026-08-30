package download.simplevpn.update

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller

/** Receives only the result token created for our own PackageInstaller session. */
class UpdateInstallReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        when (intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                @Suppress("DEPRECATION")
                val confirmation = intent.getParcelableExtra(Intent.EXTRA_INTENT) as? Intent
                if (confirmation == null) {
                    UpdateController.installerFailed(context)
                } else {
                    confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    if (runCatching { context.startActivity(confirmation) }.isFailure) {
                        UpdateController.installerFailed(context)
                    }
                }
            }

            PackageInstaller.STATUS_SUCCESS -> Unit
            else -> UpdateController.installerFailed(context)
        }
    }
}
