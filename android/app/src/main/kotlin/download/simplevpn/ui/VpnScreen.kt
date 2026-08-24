package download.simplevpn.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.platform.LocalContext
import download.simplevpn.R
import download.simplevpn.config.SliceProfileSource
import download.simplevpn.config.TransportParams
import download.simplevpn.config.XrayConfigBuilder
import download.simplevpn.core.BridgeDiagnostics
import download.simplevpn.core.EngineLog
import download.simplevpn.core.EngineSelfTest
import download.simplevpn.core.NodeReachTest
import download.simplevpn.vpn.VpnConnectionState
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.withContext

/**
 * One button and one honest line of status.
 *
 * The status never claims a connection that does not exist, and a failure shows
 * its own reason rather than a generic message: what the user should do next
 * differs between "consent denied" and "endpoint unreachable".
 */
@Composable
fun VpnScreen(
    stateFlow: StateFlow<VpnConnectionState>,
    onToggle: (isActive: Boolean) -> Unit,
) {
    val state by stateFlow.collectAsStateWithLifecycle()

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Button(
                onClick = { onToggle(state.isActive) },
                enabled = state !is VpnConnectionState.Preparing &&
                    state !is VpnConnectionState.Disconnecting,
                shape = CircleShape,
                modifier = Modifier.size(180.dp),
                colors = if (state is VpnConnectionState.Connected) {
                    ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.primary,
                    )
                } else {
                    ButtonDefaults.buttonColors(
                        containerColor = MaterialTheme.colorScheme.secondaryContainer,
                        contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
                    )
                },
            ) {
                Text(
                    text = if (state.isActive) {
                        stringResource(R.string.action_off)
                    } else {
                        stringResource(R.string.action_on)
                    },
                    style = MaterialTheme.typography.headlineMedium,
                )
            }

            Text(
                text = statusText(state),
                modifier = Modifier.padding(top = 32.dp),
                textAlign = TextAlign.Center,
                style = MaterialTheme.typography.bodyLarge,
            )

            if (state is VpnConnectionState.Connected) {
                BridgeCounters()
            }
        }
    }
}

/**
 * What the packet bridge has actually seen, refreshed while the screen is open.
 *
 * A tunnel that reports success and carries nothing looks the same from the
 * outside whatever the cause. These numbers separate the causes: no packets in
 * means the interface is not feeding the bridge, packets in with no connections
 * means they arrive and go nowhere. It is small and unexplained on purpose -
 * it is meant to be read out, not interpreted, and it disappears with the
 * slice.
 */
@Composable
private fun BridgeCounters() {
    val context = LocalContext.current
    var counters by remember { mutableStateOf(BridgeDiagnostics.snapshot()) }
    var node by remember { mutableStateOf("node: checking") }
    var engine by remember { mutableStateOf("engine: checking") }
    var engineLog by remember { mutableStateOf("log: waiting") }

    LaunchedEffect(Unit) {
        // The node first, and without the engine. A browser reaching the node
        // with the tunnel switched off proves the network allows it, not that
        // this application's own traffic escapes the tunnel it is running.
        node = withContext(Dispatchers.IO) {
            when (val profile = SliceProfileSource.load(context)) {
                is SliceProfileSource.Result.Missing -> "node: no endpoint configured"
                is SliceProfileSource.Result.Available -> {
                    val transport = profile.profile.transport
                    val serverName = when (transport) {
                        is TransportParams.VlessWsTls -> transport.serverName
                        is TransportParams.VlessReality -> transport.serverName
                    }
                    when (
                        val reach = NodeReachTest.run(
                            host = profile.profile.host,
                            port = profile.profile.port,
                            serverName = serverName,
                        )
                    ) {
                        is NodeReachTest.Result.Reached -> "node: reachable, ${reach.certificateName}"
                        is NodeReachTest.Result.Failed -> "node: unreachable, ${reach.reason}"
                    }
                }
            }
        }

        // Then the same question through the engine. Off the main thread: both
        // open sockets and wait.
        engine = when (val result = withContext(Dispatchers.IO) { EngineSelfTest.run(SOCKS_PORT) }) {
            is EngineSelfTest.Result.Reached -> "engine: reaches node, exit ${result.exitAddress}"
            is EngineSelfTest.Result.Failed -> "engine: cannot reach node, ${result.reason}"
        }
    }

    LaunchedEffect(Unit) {
        while (true) {
            counters = BridgeDiagnostics.snapshot()
            engineLog = withContext(Dispatchers.IO) { EngineLog.lastFailure(context) }
            delay(1500)
        }
    }

    Text(
        text = counters,
        modifier = Modifier.padding(top = 24.dp),
        textAlign = TextAlign.Center,
        style = MaterialTheme.typography.bodySmall,
    )

    Text(
        text = node,
        modifier = Modifier.padding(top = 12.dp),
        textAlign = TextAlign.Center,
        style = MaterialTheme.typography.bodySmall,
    )

    Text(
        text = engine,
        modifier = Modifier.padding(top = 12.dp),
        textAlign = TextAlign.Center,
        style = MaterialTheme.typography.bodySmall,
    )

    Text(
        text = engineLog,
        modifier = Modifier.padding(top = 12.dp),
        textAlign = TextAlign.Center,
        style = MaterialTheme.typography.bodySmall,
    )
}

/** Where the engine listens; kept beside the builder that puts it there. */
private const val SOCKS_PORT = XrayConfigBuilder.SOCKS_PORT

@Composable
private fun statusText(state: VpnConnectionState): String = when (state) {
    is VpnConnectionState.Disconnected -> stringResource(R.string.status_disconnected)
    is VpnConnectionState.Preparing -> stringResource(R.string.status_preparing)
    is VpnConnectionState.Connecting -> stringResource(R.string.status_connecting)
    is VpnConnectionState.Connected -> stringResource(R.string.status_connected)
    is VpnConnectionState.Reconnecting -> stringResource(R.string.status_reconnecting)
    is VpnConnectionState.Disconnecting -> stringResource(R.string.status_disconnecting)
    is VpnConnectionState.Failed -> state.reason
}
