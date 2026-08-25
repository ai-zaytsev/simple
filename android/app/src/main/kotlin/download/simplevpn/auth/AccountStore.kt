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

    val isSignedIn: Boolean get() = accountId != null

    fun remember(accountId: String) {
        prefs.edit().putString(KEY_ACCOUNT, accountId).apply()
    }

    fun forget() {
        prefs.edit().remove(KEY_ACCOUNT).apply()
    }

    private companion object {
        const val NAME = "account"
        const val KEY_ACCOUNT = "account_id"
    }
}
