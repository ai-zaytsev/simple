package download.simplevpn.metrics

import android.content.Context
import download.simplevpn.auth.AccountStore
import download.simplevpn.plan.ControlPlaneClient
import download.simplevpn.plan.PlanSource
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/** A bounded, process-scoped acceptance check of every public Control Plane entry. */
object WayInProbe {

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val running = AtomicBoolean(false)

    /** Starts in the background when due. It never holds the Activity or blocks its UI. */
    fun refresh(context: Context, force: Boolean = false) {
        val application = context.applicationContext
        if (!AccountStore(application).isSignedIn) return
        if (!running.compareAndSet(false, true)) return

        scope.launch {
            try {
                val prefs = application.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                val now = System.currentTimeMillis()
                if (!due(prefs.getLong(KEY_LAST_ACCEPTED, 0L), now, force)) return@launch

                // Learn the newest signed list first. Failure leaves the last
                // accepted descriptor in place, which is exactly what should
                // be checked during an outage.
                PlanSource(application).refreshEntries()

                val client = ControlPlaneClient(application)
                if (client.probeWaysIn() == 0 || !ServiceReport.worthSending()) return@launch

                // ServiceReport contains only counters and hosts of our own
                // entries. Core independently whitelists those hosts.
                if (client.sendReport(ServiceReport.drain())) {
                    prefs.edit().putLong(KEY_LAST_ACCEPTED, now).apply()
                }
            } finally {
                running.set(false)
            }
        }
    }

    internal fun due(lastAccepted: Long, now: Long, force: Boolean): Boolean =
        force || lastAccepted <= 0L || now < lastAccepted || now - lastAccepted >= INTERVAL_MS

    private const val PREFS = "way_in_probe"
    private const val KEY_LAST_ACCEPTED = "last_accepted"
    private const val INTERVAL_MS = 6 * 60 * 60 * 1_000L
}
