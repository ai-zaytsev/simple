package download.simplevpn.auth

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import download.simplevpn.R
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * The whole of signing in: an address, a message, a link.
 *
 * Two states, because only two things can be happening. Either the person is
 * typing an address, or a link is on its way and the application is waiting for
 * somebody to open it.
 *
 * While waiting there are exactly two ways out, and each exists because of a
 * specific way this goes wrong for real people: a message that never arrived or
 * was deleted, and an address typed with a mistake in it. Neither requires
 * restarting anything.
 *
 * The waiting state is kept on disk rather than in memory. Following the link
 * usually means picking up another device, which means leaving this one - and
 * an application in the background can be shut down at any moment. Losing the
 * wait there would break the flow at exactly the point it was designed for.
 */
@Composable
fun SignInScreen(onSignedIn: (accountId: String) -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val client = remember { AuthClient(context) }
    val accounts = remember { AccountStore(context) }
    val resumed = remember { accounts.pending() }

    var email by remember { mutableStateOf(resumed?.email ?: "") }
    var attemptId by remember { mutableStateOf(resumed?.id) }
    var message by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var resendIn by remember { mutableStateOf(0) }

    fun send() {
        scope.launch {
            busy = true
            message = null
            when (val result = withContext(Dispatchers.IO) { client.start(email) }) {
                is AuthClient.StartResult.Sent -> {
                    accounts.rememberPending(result.attemptId, email)
                    attemptId = result.attemptId
                    resendIn = result.resendAfterS
                }

                is AuthClient.StartResult.Malformed ->
                    message = context.getString(R.string.auth_bad_address)

                is AuthClient.StartResult.Unreachable ->
                    message = context.getString(R.string.auth_unreachable)
            }
            busy = false
        }
    }

    // Counts down so that the resend button becomes available when pressing it
    // might actually help. Offering it immediately would only teach people to
    // press it twice and send themselves two messages.
    LaunchedEffect(attemptId, resendIn > 0) {
        while (resendIn > 0) {
            delay(1_000)
            resendIn -= 1
        }
    }

    // Asks whether the link has been opened. It may be opened on a laptop, in
    // another browser, on another device entirely: nothing connects it to this
    // phone except this question and a row on the server.
    LaunchedEffect(attemptId) {
        val id = attemptId ?: return@LaunchedEffect
        while (true) {
            when (val result = withContext(Dispatchers.IO) { client.poll(id) }) {
                is AuthClient.PollResult.Confirmed -> {
                    accounts.clearPending()
                    accounts.remember(result.accountId)
                    onSignedIn(result.accountId)
                    return@LaunchedEffect
                }

                is AuthClient.PollResult.Expired -> {
                    accounts.clearPending()
                    attemptId = null
                    message = context.getString(R.string.auth_link_expired)
                    return@LaunchedEffect
                }

                // A momentary failure to reach the server is not a failure to
                // sign in. The link may already have been opened, so this keeps
                // asking rather than giving up on the attempt.
                is AuthClient.PollResult.Unreachable -> Unit
                is AuthClient.PollResult.Pending -> Unit
            }
            delay(POLL_INTERVAL_MS)
        }
    }

    Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(28.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            if (attemptId == null) {
                Text(
                    text = stringResource(R.string.auth_title),
                    style = MaterialTheme.typography.headlineSmall,
                    textAlign = TextAlign.Center,
                )
                Text(
                    text = stringResource(R.string.auth_explainer),
                    modifier = Modifier.padding(top = 8.dp),
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodyMedium,
                )

                OutlinedTextField(
                    value = email,
                    onValueChange = { email = it.trim() },
                    singleLine = true,
                    label = { Text(stringResource(R.string.auth_email_label)) },
                    keyboardOptions = KeyboardOptions(
                        keyboardType = KeyboardType.Email,
                        imeAction = ImeAction.Done,
                        capitalization = KeyboardCapitalization.None,
                    ),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 24.dp),
                )

                Button(
                    onClick = ::send,
                    enabled = !busy && email.isNotBlank(),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 16.dp),
                ) {
                    Text(stringResource(R.string.auth_continue))
                }
            } else {
                Text(
                    text = stringResource(R.string.auth_sent_title),
                    style = MaterialTheme.typography.headlineSmall,
                    textAlign = TextAlign.Center,
                )
                Text(
                    // Says a message was sent, never that an account exists.
                    // The application must not be usable as a way of asking
                    // whether a given person is a customer.
                    text = stringResource(R.string.auth_sent_explainer, email),
                    modifier = Modifier.padding(top = 12.dp),
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodyMedium,
                )

                TextButton(
                    onClick = ::send,
                    enabled = resendIn <= 0 && !busy,
                    modifier = Modifier.padding(top = 20.dp),
                ) {
                    Text(
                        if (resendIn > 0) {
                            stringResource(R.string.auth_resend_in, resendIn)
                        } else {
                            stringResource(R.string.auth_resend)
                        },
                    )
                }

                TextButton(
                    onClick = {
                        accounts.clearPending()
                        attemptId = null
                        message = null
                        resendIn = 0
                    },
                ) {
                    Text(stringResource(R.string.auth_change_email))
                }
            }

            message?.let {
                Text(
                    text = it,
                    modifier = Modifier.padding(top = 20.dp),
                    textAlign = TextAlign.Center,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                )
            }
        }
    }
}

private const val POLL_INTERVAL_MS = 2_500L
