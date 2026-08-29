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

        return Intent(Intent.ACTION_SENDTO).apply {
            // The address goes in the URI, which is what makes this resolve to
            // mail applications only. The subject and body go as extras: a
            // mailto: query string would need escaping that several mail
            // applications get wrong, and a mangled body is worse than none.
            data = Uri.parse("mailto:" + Uri.encode(address))
            fill(context, address)
        }
    }

    /**
     * The same letter, carrying the recording.
     *
     * The same letter is meant literally: the address, the subject and the
     * body come from the one place the support button gets them, so the two
     * cannot drift apart the next time either is edited. Somebody who records
     * a fault and somebody who describes one are writing to us about the same
     * thing and should not arrive looking like two different people.
     *
     * ACTION_SEND rather than ACTION_SENDTO, because only ACTION_SEND carries
     * an attachment - and then a selector that is a mailto: intent, which puts
     * the restriction back. Without it this would offer every application that
     * can share a file, and the recording is the one file in this application
     * that must not be one tap from a messenger: it lists the sites the phone
     * visited.
     *
     * Mail is also the channel that works. Everything else we might have used
     * is blockable from outside and has been blocked; a letter is not.
     */
    fun withRecording(context: Context, attachment: Uri): List<Intent> {
        val address = context.getString(R.string.support_email)

        // Which applications actually handle mail on this phone, asked rather
        // than described.
        //
        // The first version set a mailto: selector on an ACTION_SEND intent
        // and trusted the system to work it out. On the Business Owner's
        // phone nothing opened at all - the dialog closed and that was the
        // whole of it - and because the failure was caught and dropped, the
        // application had nothing to say about why.
        //
        // So the applications are looked up by the one intent that is known
        // to resolve here, since it is what the support button already uses,
        // and each is then addressed by name. Nothing is left to the matching
        // rules of an intent shape we cannot test from here.
        val mail = Intent(Intent.ACTION_SENDTO, Uri.parse("mailto:"))
        val handlers = runCatching {
            context.packageManager.queryIntentActivities(mail, 0)
        }.getOrDefault(emptyList())

        return handlers.mapNotNull { it.activityInfo?.packageName }.distinct().map { name ->
            Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                setPackage(name)
                fill(context, address)
                putExtra(Intent.EXTRA_STREAM, attachment)

                // Both, because the grant flag covers the extra in most
                // receivers and not in all of them. A mail application that
                // reads the URI off the clip data attaches an empty file
                // without this, which is the worst way for it to fail: it
                // looks like it worked.
                clipData = android.content.ClipData.newUri(
                    context.contentResolver,
                    context.getString(R.string.trace_share_title),
                    attachment,
                )
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
        }
    }

    private fun Intent.fill(context: Context, address: String) {
        putExtra(Intent.EXTRA_EMAIL, arrayOf(address))
        putExtra(Intent.EXTRA_SUBJECT, SupportRequest.subject(BuildConfig.VERSION_NAME))
        putExtra(Intent.EXTRA_TEXT, SupportRequest.body(facts(context)))
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
