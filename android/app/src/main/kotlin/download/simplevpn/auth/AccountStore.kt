package download.simplevpn.auth

import android.content.Context

/**
 * Whether this installation has proved who it belongs to.
 *
 * Kept per installation, not per person. Deleting the application removes it,
 * which is why a reinstall asks for the address again - and why the account it
 * then reaches is the same one, with everything attached to it.
 */
class AccountStore(context: Context) {

    private val prefs = context.getSharedPreferences(NAME, Context.MODE_PRIVATE)

    val accountId: String? get() = prefs.getString(KEY_ACCOUNT, null)

    // Signed in means holding the secret, not remembering an account. An
    // installation that kept only the account would have nothing to prove.
    val isSignedIn: Boolean get() = accountId != null && deviceToken != null

    /**
     * The secret this installation proves itself with.
     *
     * Kept beside the account rather than derived from it, because it is the
     * whole difference between being this device and merely naming it. An
     * identifier can be read off a phone and replayed by anyone; this cannot
     * be guessed, so swapping an identifier without it buys nothing.
     */
    val deviceToken: String? get() = prefs.getString(KEY_TOKEN, null)

    fun remember(accountId: String, deviceToken: String) {
        prefs.edit()
            .putString(KEY_ACCOUNT, accountId)
            .putString(KEY_TOKEN, deviceToken)
            .apply()
    }

    fun forget() {
        prefs.edit().remove(KEY_ACCOUNT).remove(KEY_TOKEN).apply()
    }

    /** A sign-in that has been started and is waiting for somebody to follow a link. */
    data class Pending(val id: String, val email: String)

    /**
     * The wait outlives the application being closed.
     *
     * Keeping this only in memory looked adequate and was not: the whole point
     * of the link is that it can be opened somewhere else, and walking to a
     * computer means leaving the application - which Android is then free to
     * shut down. Coming back to an empty address field, with a perfectly good
     * link already followed, is the flow failing at exactly the moment it was
     * designed for.
     *
     * Nothing secret is stored. The token is in the mailbox; this is only the
     * identifier of the attempt and the address already typed.
     */
    fun rememberPending(id: String, email: String) {
        prefs.edit()
            .putString(KEY_PENDING_ID, id)
            .putString(KEY_PENDING_EMAIL, email)
            .apply()
    }

    fun pending(): Pending? {
        val id = prefs.getString(KEY_PENDING_ID, null) ?: return null
        val email = prefs.getString(KEY_PENDING_EMAIL, null) ?: return null
        return Pending(id, email)
    }

    fun clearPending() {
        prefs.edit().remove(KEY_PENDING_ID).remove(KEY_PENDING_EMAIL).apply()
    }

    private companion object {
        const val NAME = "account"
        const val KEY_ACCOUNT = "account_id"
        const val KEY_TOKEN = "device_token"
        const val KEY_PENDING_ID = "pending_attempt_id"
        const val KEY_PENDING_EMAIL = "pending_email"
    }
}
