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
import download.simplevpn.auth.AccountStore
import download.simplevpn.auth.SignInScreen
import download.simplevpn.ui.VpnScreen
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
            VpnController.update(
                VpnConnectionState.Failed(getString(R.string.error_consent_denied)),
            )
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
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
            }
        }
    }

    private fun requestStart() {
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
