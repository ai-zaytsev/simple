package download.simplevpn.ui

import android.content.Intent
import android.net.Uri

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.runtime.DisposableEffect
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
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.FileProvider
import download.simplevpn.R
import download.simplevpn.core.SessionLog
import download.simplevpn.plan.ControlPlaneClient
import download.simplevpn.support.SupportMail
import android.os.SystemClock
import download.simplevpn.vpn.TraceState
import download.simplevpn.vpn.VpnConnectionState
import download.simplevpn.vpn.VpnController
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.text.NumberFormat
import java.util.Locale

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
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current

    // Which status this account is on and whether it may buy the other one,
    // asked once when the screen appears.
    //
    // Null until answered, and null again when the service cannot be reached -
    // and then the corner stays empty rather than guessing. Showing the VIP
    // offer to somebody who already has VIP would be the worse guess of the
    // two, so neither is made.
    //
    // Asked every time rather than remembered: a status changes without this
    // installation doing anything, and so does whether selling is open at all.
    var standing by remember { mutableStateOf<ControlPlaneClient.Standing?>(null) }
    var payment by remember { mutableStateOf<ControlPlaneClient.PaymentState?>(null) }
    var refreshStanding by remember { mutableStateOf(0) }
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) refreshStanding++
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }
    LaunchedEffect(refreshStanding) {
        val answer = withContext(Dispatchers.IO) {
            val client = ControlPlaneClient(context)
            client.tier() to client.currentPayment()
        }
        standing = answer.first
        payment = answer.second
    }
    val tier = standing?.tier

    var showingDevices by remember { mutableStateOf(false) }

    if (showingDevices) {
        ExternalDevicesScreen(onBack = { showingDevices = false })
        return
    }

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
        }

        // One word, in the corner, and only for an account that has it.
        //
        // Not a tab bar, not a menu, not a card on the main screen. The main
        // screen is a button and a line of status; routers and televisions are
        // something a person goes looking for once and then rarely, and
        // putting them in the way of the thing everybody opens the
        // application for would cost every user to serve some of them.
        if (tier == "VIP") {
            TextButton(
                onClick = { showingDevices = true },
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .padding(16.dp),
            ) {
                Text(text = stringResource(R.string.devices))
            }
        } else if (tier != null) {
            // The same corner, the other word.
            //
            // Always here, and disabled until it can do something. The
            // argument against a disabled control - that it promises what does
            // not exist - was made about the devices section, which a FREE
            // account never gets. This is the opposite case: VIP is something
            // this person will be able to buy, and the only question is when.
            // That is a promise with a date, and hiding it until the date
            // means the offer is only ever seen by somebody who happens to
            // open the application on the right day.
            VipButton(
                standing = standing,
                payment = payment,
                onRefresh = { refreshStanding++ },
                modifier = Modifier
                    .align(Alignment.TopEnd)
                    .padding(16.dp),
            )
        }

        TraceControls(
            connected = state is VpnConnectionState.Connected,
            modifier = Modifier
                .align(Alignment.BottomStart)
                .padding(16.dp),
        )

        // Always here, and deliberately not only when something has gone
        // wrong: the moment somebody needs to ask for help is the moment they
        // should not have to find out how.
        SupportButton(
            modifier = Modifier
                .align(Alignment.BottomEnd)
                .padding(16.dp),
        )
      }
    }
}

/**
 * Writing to us, in the person's own mail application.
 *
 * The letter is opened, not sent: it arrives in their application already
 * addressed and filled in, and they write the rest and send it themselves.
 * Nothing leaves the phone until they press send, and they see everything that
 * is in it before that happens.
 */
@Composable
private fun SupportButton(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    var noMailApplication by remember { mutableStateOf(false) }

    TextButton(
        onClick = { if (!SupportMail.open(context)) noMailApplication = true },
        modifier = modifier,
    ) {
        Text(text = stringResource(R.string.support))
    }

    // A phone with no mail application is unusual and not impossible, and the
    // person on it still needs to be able to write to us.
    if (noMailApplication) {
        AlertDialog(
            onDismissRequest = { noMailApplication = false },
            title = { Text(text = stringResource(R.string.support_no_mail_title)) },
            text = {
                Text(
                    text = stringResource(
                        R.string.support_no_mail_body,
                        stringResource(R.string.support_email),
                    ),
                )
            },
            confirmButton = {
                TextButton(onClick = { noMailApplication = false }) {
                    Text(text = stringResource(R.string.support_no_mail_close))
                }
            },
        )
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
    var saveOutcome by remember { mutableStateOf(0) }
    var noMailApplication by remember { mutableStateOf(false) }

    // Where to save is the system's question, not ours. A picker everybody has
    // already used beats any folder we could choose for them, and it is the
    // difference between "saved" and "saved somewhere they will have to find".
    val saveTo = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("text/plain"),
    ) { destination ->
        if (destination == null) return@rememberLauncherForActivityResult
        scope.launch {
            val ok = withContext(Dispatchers.IO) {
                runCatching {
                    val file = SessionLog.exportTrace(context) ?: return@runCatching false
                    context.contentResolver.openOutputStream(destination)?.use { out ->
                        file.inputStream().use { it.copyTo(out) }
                    } ?: return@runCatching false
                    true
                }.getOrDefault(false)
            }
            saveOutcome = if (ok) R.string.trace_saved else R.string.trace_save_failed

            // Removed only once it is somewhere else. The send path can drop
            // it as soon as the letter is handed over, because the letter
            // carries it; this one has to wait for the copy to have worked, or
            // a failed save would take the recording with it.
            if (ok) {
                withContext(Dispatchers.IO) { SessionLog.dropTrace(context) }
                VpnController.traceCleared()
            }
        }
    }

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

            // Offered only while there is a tunnel, and absent otherwise
            // rather than greyed out: with no engine running there is nothing
            // to record, and a control that cannot do anything is better not
            // shown than shown and explained.
            is TraceState.Idle -> if (connected) {
                TextButton(onClick = { askingToStart = true }) {
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
        AlertDialog(
            onDismissRequest = { askingToSend = false },
            title = { Text(text = stringResource(R.string.trace_send_title)) },
            // The number is the point. "May contain information about services
            // you used" is understood by nobody; "74 sites" is understood by
            // everybody, immediately.
            //
            // And nothing else. Saving instead of sending is what somebody
            // does when support has asked them to attach the file to a letter
            // they already sent - so the instruction arrives from a person,
            // and a paragraph here explaining it would be a paragraph read by
            // everybody to serve the few who were told to look for it.
            text = { Text(text = stringResource(R.string.trace_send_body, destinations)) },
            // All three in one column, and no dismiss slot at all.
            //
            // Split across the two slots, Material lays the confirm button out
            // beside or above the dismiss group and stretches the gaps to fill
            // the dialog: the three ended up scattered down the card with the
            // last one cut off by its bottom edge. A dialog with three choices
            // is a list of three choices, not a confirm and a pair.
            confirmButton = {
                Column(horizontalAlignment = Alignment.End) {
                    TextButton(
                        onClick = {
                            askingToSend = false
                            scope.launch {
                                if (!sendRecordingByMail(context)) noMailApplication = true
                            }
                        },
                    ) {
                        Text(text = stringResource(R.string.trace_send_confirm))
                    }
                    TextButton(
                        onClick = {
                            askingToSend = false
                            // Named for the person, not for the file. The
                            // system picker asks where; this only has to say
                            // what it is called.
                            saveTo.launch("simple-vpn-log.txt")
                        },
                    ) {
                        Text(text = stringResource(R.string.trace_send_save))
                    }
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
                }
            },
        )
    }

    // Nothing opened, and the person is told so.
    //
    // This is the defect that mattered: the letter failed to open, the failure
    // was caught and dropped, and the dialog simply closed. An application
    // that does nothing and says nothing is indistinguishable from one that is
    // broken - and the recording is still here, which is the part somebody
    // needs to know before they go and make another one.
    if (noMailApplication) {
        AlertDialog(
            onDismissRequest = { noMailApplication = false },
            title = { Text(text = stringResource(R.string.support_no_mail_title)) },
            text = {
                Text(
                    text = stringResource(
                        R.string.trace_no_mail_body,
                        stringResource(R.string.support_email),
                    ),
                )
            },
            confirmButton = {
                TextButton(onClick = { noMailApplication = false }) {
                    Text(text = stringResource(R.string.support_no_mail_close))
                }
            },
        )
    }

    if (saveOutcome != 0) {
        AlertDialog(
            onDismissRequest = { saveOutcome = 0 },
            text = { Text(text = stringResource(saveOutcome)) },
            confirmButton = {
                TextButton(onClick = { saveOutcome = 0 }) {
                    Text(text = stringResource(R.string.support_no_mail_close))
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

/**
 * Sends the recording as the same letter the support button writes.
 *
 * Not a chooser. The first version offered every application that can share a
 * file, which was wrong twice over: the recording lists the sites this phone
 * visited and should not be one tap from a messenger, and mail is the channel
 * that actually works - everything else we might have used is blockable from
 * outside and has been blocked. A letter is not.
 *
 * The address, the subject and the body come from SupportMail, which is to say
 * from the same lines the support button uses. Somebody who records a fault
 * and somebody who describes one are writing about the same thing and should
 * not arrive looking like two different people.
 */
private suspend fun sendRecordingByMail(context: android.content.Context): Boolean {
    val file = withContext(Dispatchers.IO) { SessionLog.exportTrace(context) } ?: return false
    val uri = FileProvider.getUriForFile(context, "${context.packageName}.logs", file)

    val letters = SupportMail.withRecording(context, uri)
    if (letters.isEmpty()) return false

    val opened = runCatching {
        // One mail application: straight there. Several: the system asks
        // which, and the list holds only the ones asked for by name, so a
        // messenger cannot appear in it.
        val start = if (letters.size == 1) {
            letters.first()
        } else {
            Intent.createChooser(letters.first(), null).apply {
                putExtra(
                    Intent.EXTRA_INITIAL_INTENTS,
                    letters.drop(1).toTypedArray(),
                )
            }
        }
        context.startActivity(start.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }.isSuccess

    // Kept when nothing took it. Dropping the recording after a letter that
    // never opened would destroy the one thing the person spent five minutes
    // producing, and leave them with nothing to try again with.
    if (opened) {
        withContext(Dispatchers.IO) { SessionLog.dropTrace(context) }
        VpnController.traceCleared()
    }
    return opened
}

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

/**
 * The VIP offer, in the corner a VIP account keeps its devices in.
 *
 * Visible always, and doing something only when the service says it may. The
 * reason is always shown, because a control that refuses without saying why is
 * the thing people write to support about - and the three reasons need three
 * different answers:
 *
 * The wait has a date, and the date is the whole of the message: nothing is
 * wrong, come back then.
 *
 * Sales being off has no date, because nobody has decided one. Inventing
 * "soon" would be a promise made by a screen rather than by a person.
 *
 * There is no third refusal for an account that already has VIP: that account
 * sees the devices section here instead, which is what it wanted from VIP in
 * the first place.
 */
@Composable
private fun VipButton(
    standing: ControlPlaneClient.Standing?,
    payment: ControlPlaneClient.PaymentState?,
    onRefresh: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var explaining by remember { mutableStateOf(false) }
    var choosing by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    var failed by remember { mutableStateOf(false) }
    // "creating" means Core has a retryable idempotent operation but no
    // checkout yet, so choosing the same product retries it. Only a real
    // provider checkout is presented as pending.
    val pending = payment?.status == "pending"

    TextButton(
        onClick = {
            when {
                standing?.mayBuy != true -> explaining = true
                pending -> explaining = true
                else -> choosing = true
            }
        },
        modifier = modifier,
        // Not disabled in the Compose sense. A greyed-out control cannot be
        // pressed, so it cannot explain itself either, and the person is left
        // with a word and no way to ask about it. It is enabled and it answers.
    ) {
        Text(text = stringResource(R.string.vip))
    }

    if (explaining) {
        val buy = standing?.mayBuy == true
        AlertDialog(
            onDismissRequest = { explaining = false },
            title = { Text(text = stringResource(R.string.vip)) },
            text = {
                Text(
                    text = when {
                        pending -> stringResource(R.string.vip_payment_pending)
                        buy -> stringResource(R.string.vip_soon)
                        standing?.whyNot == "too_soon" && standing.opensOn.isNotBlank() ->
                            stringResource(R.string.vip_wait, readableDay(standing.opensOn))
                        standing?.whyNot == "too_soon" -> stringResource(R.string.vip_wait_unknown)
                        else -> stringResource(R.string.vip_closed)
                    },
                )
            },
            confirmButton = {
                Column(horizontalAlignment = Alignment.End) {
                    if (pending && payment?.checkoutUrl?.isNotBlank() == true) {
                        TextButton(onClick = {
                            explaining = false
                            if (!openCheckout(context, payment.checkoutUrl)) failed = true
                        }) {
                            Text(text = stringResource(R.string.vip_payment_continue))
                        }
                        TextButton(onClick = {
                            explaining = false
                            onRefresh()
                        }) {
                            Text(text = stringResource(R.string.vip_payment_check))
                        }
                    }
                    TextButton(onClick = { explaining = false }) {
                        Text(text = stringResource(R.string.support_no_mail_close))
                    }
                }
            },
        )
    }

    if (choosing) {
        AlertDialog(
            onDismissRequest = { if (!busy) choosing = false },
            title = { Text(text = stringResource(R.string.vip_choose)) },
            text = {
                Column {
                    standing?.products.orEmpty().forEach { product ->
                        TextButton(
                            onClick = {
                                if (busy) return@TextButton
                                busy = true
                                scope.launch {
                                    val started = withContext(Dispatchers.IO) {
                                        ControlPlaneClient(context).startPayment(product.id)
                                    }
                                    busy = false
                                    choosing = false
                                    if (started == null || !openCheckout(context, started.checkoutUrl)) {
                                        failed = true
                                    }
                                }
                            },
                            enabled = !busy,
                        ) {
                            Text(text = "${product.title} · ${rubles(product.amountMinor)}")
                        }
                    }
                }
            },
            confirmButton = {},
            dismissButton = {
                TextButton(onClick = { choosing = false }, enabled = !busy) {
                    Text(text = stringResource(R.string.devices_cancel))
                }
            },
        )
    }

    if (failed) {
        AlertDialog(
            onDismissRequest = { failed = false },
            text = { Text(text = stringResource(R.string.vip_payment_failed)) },
            confirmButton = {
                TextButton(onClick = { failed = false }) {
                    Text(text = stringResource(R.string.support_no_mail_close))
                }
            },
        )
    }
}

private fun openCheckout(context: android.content.Context, address: String): Boolean {
    val uri = runCatching { Uri.parse(address) }.getOrNull() ?: return false
    if (uri.scheme != "https" || uri.host.isNullOrBlank()) return false
    return runCatching {
        context.startActivity(Intent(Intent.ACTION_VIEW, uri))
    }.isSuccess
}

private fun rubles(amountMinor: Long): String {
    val whole = amountMinor / 100
    return NumberFormat.getIntegerInstance(Locale("ru", "RU")).format(whole) + " ₽"
}

/**
 * The day out of a timestamp, without pretending to do arithmetic on it.
 *
 * The server decides when the wait ends and sends the moment; this only makes
 * it readable. Counting days here would mean trusting the phone's clock, and a
 * phone whose clock is a week fast would announce that the wait is over while
 * the service goes on refusing - the application and the server disagreeing in
 * front of the person.
 */
private fun readableDay(timestamp: String): String {
    val day = timestamp.substringBefore('T')
    val parts = day.split('-')
    return if (parts.size == 3) "${parts[2]}.${parts[1]}.${parts[0]}" else day
}
