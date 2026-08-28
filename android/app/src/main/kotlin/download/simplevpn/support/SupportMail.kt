package download.simplevpn.support

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import download.simplevpn.BuildConfig
import download.simplevpn.R
import download.simplevpn.auth.DeviceIdentity

/**
 * Hands the letter to whatever mail application the person uses.
 *
 * ACTION_SENDTO with a mailto: address rather than ACTION_SEND, and the
 * difference is not cosmetic: ACTION_SEND offers every application that can
 * share text - messengers, notes, clipboards - and a support message full of
 * device facts should not be one tap away from landing in one of those.
 * SENDTO with mailto: is answered by mail applications and nothing else.
 *
 * Nothing is sent by us. The letter opens in their application, already
 * addressed and filled in, and they write the rest and press send themselves.
 */
object SupportMail {

    fun intent(context: Context): Intent {
        val address = context.getString(R.string.support_email)
        val facts = facts(context)

        return Intent(Intent.ACTION_SENDTO).apply {
            // The address goes in the URI, which is what makes this resolve to
            // mail applications only. The subject and body go as extras: a
            // mailto: query string would need escaping that several mail
            // applications get wrong, and a mangled body is worse than none.
            data = Uri.parse("mailto:" + Uri.encode(address))
            putExtra(Intent.EXTRA_EMAIL, arrayOf(address))
            putExtra(Intent.EXTRA_SUBJECT, SupportRequest.subject(BuildConfig.VERSION_NAME))
            putExtra(Intent.EXTRA_TEXT, SupportRequest.body(facts))
        }
    }

    /**
     * Opens the letter, and says whether anything took it.
     *
     * The button is shown whichever way this goes. Hiding it when no mail
     * application answers would leave somebody with a broken connection and no
     * visible way to say so; the screen tells them the address instead, which
     * they can use from anywhere.
     */
    fun open(context: Context): Boolean = runCatching {
        context.startActivity(intent(context).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }.isSuccess

    private fun facts(context: Context): SupportRequest.Facts {
        val recorded = LastError.read(context)
        return SupportRequest.Facts(
            appVersion = BuildConfig.VERSION_NAME,
            deviceModel = Build.MANUFACTURER + " " + Build.MODEL,
            androidVersion = Build.VERSION.RELEASE + " (API " + Build.VERSION.SDK_INT + ")",
            deviceId = DeviceIdentity.of(context).deviceId,
            lastError = recorded?.reason,
            lastErrorAgo = recorded?.let { LastError.ago(it.atMillis, System.currentTimeMillis()) },
        )
    }
}
