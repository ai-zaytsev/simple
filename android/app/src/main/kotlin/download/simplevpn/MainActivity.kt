package download.simplevpn

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.material3.MaterialTheme
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
                VpnScreen(
                    stateFlow = VpnController.state,
                    onToggle = { isActive -> if (isActive) requestStop() else requestStart() },
                )
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
