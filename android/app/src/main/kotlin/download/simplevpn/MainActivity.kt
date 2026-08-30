package download.simplevpn

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import download.simplevpn.auth.AccountStore
import download.simplevpn.support.LastError
import download.simplevpn.auth.SignInScreen
import download.simplevpn.ui.VpnScreen
import download.simplevpn.update.AppUpdateDialog
import download.simplevpn.update.AppUpdatePolicy
import download.simplevpn.update.UpdateController
import download.simplevpn.vpn.VpnConnectionState
import download.simplevpn.vpn.VpnController

/**
 * The only screen.
 *
 * It holds no connection state of its own: it observes [VpnController] and
 * sends intents. Destroying it has no effect on an established tunnel, which is
 * why the tunnel lives in the service and the state object is process-scoped.
 */
class MainActivity : ComponentActivity() {

    private val consentLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            VpnController.start(this)
        } else {
            val reason = getString(R.string.error_consent_denied)
            LastError.record(this, reason)
            VpnController.update(VpnConnectionState.Failed(reason))
        }
    }

    /**
     * Somebody is looking at the application, so whatever the service would
     * have asked at its next tick, ask it now.
     *
     * This is the moment an answer is worth having: a person whose connection
     * has stopped working opens the application to find out why. Shortening the
     * timer instead would cost battery on every phone, awake or not, for the
     * sake of the rare minute when somebody is actually watching.
     */
    override fun onResume() {
        super.onResume()
        UpdateController.refresh(this)
        VpnController.recheck(this)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        // A finished recording belongs to the person who deliberately made
        // it. Recreating the screen or the process must not take away their
        // chance to send, save or delete it. An active in-process recording is
        // left untouched; a file from a stopped process becomes Ready.
        VpnController.restoreTrace(this)

        setContent {
            MaterialTheme {
                val update by UpdateController.state.collectAsStateWithLifecycle()
                // Which screen to show is decided by whether this installation
                // has proved access to a mailbox, and by nothing else. There is
                // no password to remember, so there is nothing to be logged out
                // of: an installation is either confirmed or it is new.
                val accounts = remember { AccountStore(this) }
                var signedIn by remember { mutableStateOf(accounts.isSignedIn) }

                if (signedIn) {
                    // The service can sign this installation out on its own,
                    // when the server stops recognising it - somebody signed in
                    // elsewhere, or this device was cut off. It happens in the
                    // background, so this screen has to notice rather than be
                    // told, and every change of connection state is a moment
                    // when it might have happened.
                    LaunchedEffect(Unit) {
                        VpnController.state.collect {
                            if (!accounts.isSignedIn) signedIn = false
                        }
                    }

                    VpnScreen(
                        stateFlow = VpnController.state,
                        onToggle = { isActive -> if (isActive) requestStop() else requestStart() },
                    )
                } else {
                    SignInScreen(onSignedIn = { signedIn = true })
                }

                AppUpdateDialog(
                    state = update,
                    onUpdate = { UpdateController.begin(this) },
                    onLater = { UpdateController.dismissOptional() },
                )
            }
        }
    }

    private fun requestStart() {
        if (UpdateController.state.value.verdict is AppUpdatePolicy.Verdict.Required) return
        // The system asks the user once per install. prepare() returns null
        // when consent already exists.
        val consentIntent: Intent? = VpnService.prepare(this)
        if (consentIntent == null) {
            VpnController.start(this)
        } else {
            VpnController.update(VpnConnectionState.Preparing)
            consentLauncher.launch(consentIntent)
        }
    }

    private fun requestStop() {
        VpnController.stop(this)
    }
}
