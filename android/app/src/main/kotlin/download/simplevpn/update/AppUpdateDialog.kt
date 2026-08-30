package download.simplevpn.update

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import download.simplevpn.R

@Composable
fun AppUpdateDialog(
    state: UpdateController.State,
    onUpdate: () -> Unit,
    onLater: () -> Unit,
) {
    if (!state.visible) return

    AlertDialog(
        onDismissRequest = { if (!state.required) onLater() },
        title = {
            Text(
                text = stringResource(
                    if (state.required) R.string.update_required_title else R.string.update_available_title,
                ),
            )
        },
        text = {
            Text(
                text = when {
                    state.phase == UpdateController.Phase.NEEDS_PERMISSION ->
                        stringResource(R.string.update_permission)
                    state.phase == UpdateController.Phase.DOWNLOADING ->
                        stringResource(R.string.update_downloading)
                    state.phase == UpdateController.Phase.INSTALLING ->
                        stringResource(R.string.update_installing)
                    state.failure == UpdateFailure.HASH_MISMATCH ->
                        stringResource(R.string.update_hash_failed)
                    state.failure == UpdateFailure.INSTALLER ->
                        stringResource(R.string.update_install_failed)
                    state.failure != null -> stringResource(R.string.update_download_failed)
                    state.required -> stringResource(R.string.update_required_body)
                    else -> stringResource(R.string.update_available_body)
                },
            )
        },
        confirmButton = {
            TextButton(onClick = onUpdate, enabled = !state.busy) {
                Text(
                    text = stringResource(
                        if (state.phase == UpdateController.Phase.NEEDS_PERMISSION) {
                            R.string.update_open_settings
                        } else {
                            R.string.update_now
                        },
                    ),
                )
            }
        },
        dismissButton = if (state.required) {
            null
        } else {
            {
                TextButton(onClick = onLater, enabled = !state.busy) {
                    Text(text = stringResource(R.string.update_later))
                }
            }
        },
    )
}
