package download.simplevpn.ui

import android.content.Intent

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.FileProvider
import download.simplevpn.R
import download.simplevpn.config.TransportParams
import download.simplevpn.config.XrayConfigBuilder
import download.simplevpn.core.BridgeDiagnostics
import download.simplevpn.core.SessionLog
import download.simplevpn.core.EngineSelfTest
import download.simplevpn.core.NodeReachTest
import download.simplevpn.plan.PlanSource
import android.os.SystemClock
import download.simplevpn.vpn.TraceState
import download.simplevpn.vpn.VpnConnectionState
import download.simplevpn.vpn.VpnController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
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
      Box(modifier = Modifier.fillMaxSize()) {
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

            // Outside the connected branch on purpose: the interesting log is
            // usually the one from a session that has just ended.
            ShareSessionLog()
        }

        // In the corner rather than beside the ordinary log button, because
        // these are different things and putting them together is how somebody
        // sends the wrong one. The everyday log is safe and sits with the
        // status; the recording is not, and stands apart.
        TraceControls(
            connected = state is VpnConnectionState.Connected,
            modifier = Modifier
                .align(Alignment.BottomStart)
                .padding(16.dp),
        )
      }
    }
}

/**
 * The detailed recording: starting it, seeing that it is running, sending it.
 *
 * Everything here exists because of one incident. A session log was sent in to
 * diagnose an unrelated fault; it listed seventy-four sites the phone had
 * visited in under four minutes, and the person who sent it did not know that.
 * The recording had been running the whole time, unasked for and unannounced.
 *
 * So: it does not run unless somebody starts it, it says what it holds before
 * it starts, it is visible while it runs, it stops itself, and what it holds is
 * counted and shown before it can be sent.
 */
@Composable
private fun TraceControls(connected: Boolean, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val trace by VpnController.trace.collectAsStateWithLifecycle()

    var askingToStart by remember { mutableStateOf(false) }
    var askingToSend by remember { mutableStateOf(false) }
    var destinations by remember { mutableStateOf(0) }

    Column(modifier = modifier) {
        when (val current = trace) {
            is TraceState.Recording -> {
                RecordingIndicator(stopsAtElapsedMillis = current.stopsAtElapsedMillis)
                TextButton(onClick = { VpnController.stopTrace(context) }) {
                    Text(text = stringResource(R.string.trace_stop))
                }
            }

            is TraceState.Ready -> {
                TextButton(
                    onClick = {
                        scope.launch {
                            destinations = withContext(Dispatchers.IO) {
                                SessionLog.traceDestinations(context)
                            }
                            askingToSend = true
                        }
                    },
                ) {
                    Text(text = stringResource(R.string.trace_send))
                }
            }

            is TraceState.Idle -> {
                TextButton(
                    onClick = { askingToStart = true },
                    enabled = connected,
                ) {
                    Text(text = stringResource(R.string.trace_start))
                }
            }
        }
    }

    if (askingToStart) {
        AlertDialog(
            onDismissRequest = { askingToStart = false },
            title = { Text(text = stringResource(R.string.trace_warning_title)) },
            text = { Text(text = stringResource(R.string.trace_warning_body)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        askingToStart = false
                        VpnController.startTrace(context)
                    },
                ) {
                    Text(text = stringResource(R.string.trace_warning_confirm))
                }
            },
            dismissButton = {
                TextButton(onClick = { askingToStart = false }) {
                    Text(text = stringResource(R.string.trace_warning_cancel))
                }
            },
        )
    }

    if (askingToSend) {
        val title = stringResource(R.string.trace_share_title)
        AlertDialog(
            onDismissRequest = { askingToSend = false },
            title = { Text(text = stringResource(R.string.trace_send_title)) },
            // The number is the point. "May contain information about services
            // you used" is understood by nobody; "74 sites" is understood by
            // everybody, immediately.
            text = { Text(text = stringResource(R.string.trace_send_body, destinations)) },
            confirmButton = {
                TextButton(
                    onClick = {
                        askingToSend = false
                        scope.launch {
                            val file = withContext(Dispatchers.IO) {
                                SessionLog.exportTrace(context)
                            } ?: return@launch
                            shareFile(context, file, title)

                            // Gone from the device the moment it has been
                            // handed over. Whoever holds it now holds the only
                            // copy, which is the person who chose to.
                            withContext(Dispatchers.IO) { SessionLog.dropTrace(context) }
                            VpnController.traceCleared()
                        }
                    },
                ) {
                    Text(text = stringResource(R.string.trace_send_confirm))
                }
            },
            dismissButton = {
                TextButton(
                    onClick = {
                        askingToSend = false
                        scope.launch {
                            withContext(Dispatchers.IO) { SessionLog.dropTrace(context) }
                            VpnController.traceCleared()
                        }
                    },
                ) {
                    Text(text = stringResource(R.string.trace_send_delete))
                }
            },
        )
    }
}

/** Says that a recording is running, and how much of it is left. */
@Composable
private fun RecordingIndicator(stopsAtElapsedMillis: Long) {
    var remaining by remember(stopsAtElapsedMillis) { mutableStateOf(0L) }

    LaunchedEffect(stopsAtElapsedMillis) {
        while (true) {
            remaining = ((stopsAtElapsedMillis - SystemClock.elapsedRealtime()) / 1000)
                .coerceAtLeast(0)
            delay(1000)
        }
    }

    Text(
        text = stringResource(R.string.trace_running, remaining / 60, remaining % 60),
        color = MaterialTheme.colorScheme.error,
        style = MaterialTheme.typography.bodyMedium,
    )
}

/** Hands a file to whatever the user picks to send it with. */
private fun shareFile(context: android.content.Context, file: java.io.File, title: String) {
    val uri = FileProvider.getUriForFile(context, "${context.packageName}.logs", file)
    val send = Intent(Intent.ACTION_SEND).apply {
        type = "text/plain"
        putExtra(Intent.EXTRA_STREAM, uri)
        putExtra(Intent.EXTRA_SUBJECT, title)
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    runCatching {
        context.startActivity(
            Intent.createChooser(send, title).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
        )
    }
}

/**
 * Hands the whole session log to whatever the user picks to send it with.
 *
 * Reading counters off a screen located the broken layer but never the reason,
 * and every guess cost an install. The file says what the application did, in
 * order, and what the engine said about it.
 *
 * Diagnostic, and it leaves the device only when the user taps this. The engine
 * writes at a level that names destinations, so this button and the level that
 * fills it go together when the slice does.
 */
@Composable
private fun ShareSessionLog() {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val title = stringResource(R.string.share_log_title)

    TextButton(
        onClick = {
            scope.launch {
                val file = withContext(Dispatchers.IO) { SessionLog.export(context) } ?: return@launch
                val uri = FileProvider.getUriForFile(context, "${context.packageName}.logs", file)
                val send = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_STREAM, uri)
                    putExtra(Intent.EXTRA_SUBJECT, title)
                    addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
                }
                runCatching {
                    context.startActivity(
                        Intent.createChooser(send, title)
                            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                    )
                }
            }
        },
        modifier = Modifier.padding(top = 20.dp),
    ) {
        Text(text = stringResource(R.string.action_share_log))
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
            when (val known = PlanSource(context).currentProfile()) {
                is PlanSource.Result.Missing -> "node: ${known.reason}"
                is PlanSource.Result.Revoked -> "node: this installation is not recognised"
                is PlanSource.Result.Available -> {
                    val serverName = when (val transport = known.profile.transport) {
                        is TransportParams.VlessWsTls -> transport.serverName
                        is TransportParams.VlessReality -> transport.serverName
                    }
                    when (
                        val reach = NodeReachTest.run(
                            host = known.profile.host,
                            port = known.profile.port,
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
            engineLog = withContext(Dispatchers.IO) { SessionLog.lastFailure(context) }
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
