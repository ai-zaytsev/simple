package download.simplevpn.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.unit.dp
import download.simplevpn.R
import download.simplevpn.plan.ControlPlaneClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject

/**
 * One router, television or computer, and the one address it uses.
 *
 * One, not a list. The first version handed back a link per node, so that a
 * client holding several would survive one of them being retired. With a
 * hundred nodes that is a hundred links for one television, and nobody pastes
 * a hundred of anything. Somebody wanting a second connection adds a second
 * device and names it.
 *
 * Empty when the service could not build one just now. That row still has to
 * appear: it is the one its owner presses "new link" on.
 */
data class ExternalDevice(
    val id: String,
    val label: String,
    val link: String,
)

/**
 * Routers, televisions and everything else that is not this application.
 *
 * A separate screen, reached from one word on the main one, and only by an
 * account that has it. The main screen is a button and a line of status, and
 * this stage is not a reason to make it anything else: somebody who only wants
 * the tunnel on should not have to look past a list of televisions to find it.
 *
 * Every device here has its own access. That is the shape of stage 22 and it
 * is what makes the last row of buttons honest - revoking one television
 * cannot take the router with it, because they never shared anything.
 */
@Composable
fun ExternalDevicesScreen(onBack: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val clipboard = LocalClipboardManager.current

    val client = remember { ControlPlaneClient(context) }

    var devices by remember { mutableStateOf<List<ExternalDevice>>(emptyList()) }
    var loaded by remember { mutableStateOf(false) }
    var unreachable by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }

    var naming by remember { mutableStateOf(false) }
    var replacing by remember { mutableStateOf<ExternalDevice?>(null) }
    var revoking by remember { mutableStateOf<ExternalDevice?>(null) }
    var copied by remember { mutableStateOf(false) }

    // Reading the list opens a connection, so it never happens on the thread
    // that draws. Written once here and reused by every action below, because
    // an action that changes the list has to leave the screen showing what the
    // service now holds rather than what the phone last guessed.
    suspend fun reload() {
        val answer = withContext(Dispatchers.IO) { client.externalDevices() }
        if (answer !is ControlPlaneClient.Result.Received) {
            unreachable = true
            loaded = true
            return
        }
        unreachable = false
        devices = readDevices(answer.envelopeJson)
        loaded = true
    }

    LaunchedEffect(Unit) { reload() }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(modifier = Modifier.fillMaxSize().padding(24.dp)) {
            Text(
                text = stringResource(R.string.devices_title),
                style = MaterialTheme.typography.headlineSmall,
            )
            Text(
                text = stringResource(R.string.devices_explain),
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(top = 8.dp),
            )

            if (unreachable) {
                Text(
                    text = stringResource(R.string.devices_unreachable),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(top = 24.dp),
                )
            } else if (loaded && devices.isEmpty()) {
                Text(
                    text = stringResource(R.string.devices_empty),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.padding(top = 24.dp),
                )
            }

            LazyColumn(modifier = Modifier.weight(1f).padding(top = 16.dp)) {
                items(devices, key = { it.id }) { device ->
                    DeviceRow(
                        device = device,
                        enabled = !busy,
                        onCopy = {
                            clipboard.setText(AnnotatedString(device.link))
                            copied = true
                        },
                        onReplace = { replacing = device },
                        onRevoke = { revoking = device },
                    )
                    HorizontalDivider()
                }
            }

            Row(
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                TextButton(onClick = onBack) {
                    Text(text = stringResource(R.string.devices_back))
                }
                Button(onClick = { naming = true }, enabled = !busy) {
                    Text(text = stringResource(R.string.devices_add))
                }
            }
        }
    }

    if (copied) {
        AlertDialog(
            onDismissRequest = { copied = false },
            text = { Text(text = stringResource(R.string.devices_copied)) },
            confirmButton = {
                TextButton(onClick = { copied = false }) {
                    Text(text = stringResource(R.string.devices_back))
                }
            },
        )
    }

    if (naming) {
        var label by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { naming = false },
            title = { Text(text = stringResource(R.string.devices_name_title)) },
            text = {
                OutlinedTextField(
                    value = label,
                    onValueChange = { label = it },
                    singleLine = true,
                    placeholder = { Text(text = stringResource(R.string.devices_name_hint)) },
                )
            },
            confirmButton = {
                // Refused while the name is empty rather than accepted and
                // named for them. A list of devices called "Устройство 1" is a
                // list nobody can revoke the right row from.
                TextButton(
                    enabled = label.isNotBlank(),
                    onClick = {
                        naming = false
                        busy = true
                        scope.launch {
                            withContext(Dispatchers.IO) { client.addExternalDevice(label.trim()) }
                            reload()
                            busy = false
                        }
                    },
                ) {
                    Text(text = stringResource(R.string.devices_name_save))
                }
            },
            dismissButton = {
                TextButton(onClick = { naming = false }) {
                    Text(text = stringResource(R.string.devices_cancel))
                }
            },
        )
    }

    replacing?.let { device ->
        AlertDialog(
            onDismissRequest = { replacing = null },
            title = { Text(text = stringResource(R.string.devices_replace_title)) },
            // Said before it happens, because it cannot be undone and because
            // the person doing it is usually mid-way through fixing something
            // else. A television that stops working twice in one evening is
            // worse than one that stops once.
            text = { Text(text = stringResource(R.string.devices_replace_body)) },
            confirmButton = {
                TextButton(onClick = {
                    replacing = null
                    busy = true
                    scope.launch {
                        withContext(Dispatchers.IO) { client.replaceExternalLink(device.id) }
                        reload()
                        busy = false
                    }
                }) {
                    Text(text = stringResource(R.string.devices_replace_yes))
                }
            },
            dismissButton = {
                TextButton(onClick = { replacing = null }) {
                    Text(text = stringResource(R.string.devices_cancel))
                }
            },
        )
    }

    revoking?.let { device ->
        AlertDialog(
            onDismissRequest = { revoking = null },
            title = { Text(text = stringResource(R.string.devices_revoke_title)) },
            text = { Text(text = stringResource(R.string.devices_revoke_body)) },
            confirmButton = {
                TextButton(onClick = {
                    revoking = null
                    busy = true
                    scope.launch {
                        withContext(Dispatchers.IO) { client.revokeDevice(device.id) }
                        reload()
                        busy = false
                    }
                }) {
                    Text(text = stringResource(R.string.devices_revoke_yes))
                }
            },
            dismissButton = {
                TextButton(onClick = { revoking = null }) {
                    Text(text = stringResource(R.string.devices_cancel))
                }
            },
        )
    }
}

@Composable
private fun DeviceRow(
    device: ExternalDevice,
    enabled: Boolean,
    onCopy: () -> Unit,
    onReplace: () -> Unit,
    onRevoke: () -> Unit,
) {
    Column(modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp)) {
        Text(text = device.label, style = MaterialTheme.typography.titleMedium)

        // The address itself, shortened. Shown rather than only copied,
        // because a person setting up a router needs to see that the thing
        // exists before trusting a button to have put it somewhere - and
        // because two devices should visibly differ.
        Text(
            text = if (device.link.isBlank()) {
                stringResource(R.string.devices_no_link)
            } else {
                device.link.take(28) + "…"
            },
            style = MaterialTheme.typography.bodySmall,
        )
        // Scrollable sideways, and that is not decoration.
        //
        // The first version put three buttons in a plain Row and the third
        // one - delete - fell off the edge of the screen. Not greyed out, not
        // cut in half: absent. The Business Owner asked where deleting a
        // device was, and the answer was that it had been there all along,
        // eleven pixels past the right margin.
        //
        // A row that can scroll cannot hide its last child. The labels are
        // short enough to fit on an ordinary phone anyway; this is what
        // happens on the phone that is not ordinary.
        Row(
            modifier = Modifier
                .padding(top = 4.dp)
                .horizontalScroll(rememberScrollState()),
        ) {
            TextButton(onClick = onCopy, enabled = enabled && device.link.isNotBlank()) {
                Text(text = stringResource(R.string.devices_copy))
            }
            TextButton(onClick = onReplace, enabled = enabled) {
                Text(text = stringResource(R.string.devices_replace))
            }
            TextButton(onClick = onRevoke, enabled = enabled) {
                Text(text = stringResource(R.string.devices_revoke))
            }
        }
    }
}

/**
 * The answer, read defensively.
 *
 * A device with no links is still a device and still has to appear: it is the
 * row somebody presses "новая ссылка" on. Dropping it because its list came
 * back empty would hide the one device that needs attention.
 */
private fun readDevices(json: String): List<ExternalDevice> {
    val out = mutableListOf<ExternalDevice>()
    val array = JSONObject(json).optJSONArray("devices") ?: return out
    for (i in 0 until array.length()) {
        val item = array.optJSONObject(i) ?: continue
        val id = item.optString("device_id")
        if (id.isBlank()) continue
        out.add(
            ExternalDevice(
                id = id,
                label = item.optString("label"),
                link = item.optString("link"),
            )
        )
    }
    return out
}
