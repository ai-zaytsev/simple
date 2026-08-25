package download.simplevpn.auth

import android.content.Context
import java.util.UUID

/**
 * What this installation calls itself.
 *
 * Generated once and kept, never derived from anything about the hardware. An
 * identifier tied to the device would follow the person across reinstalls and
 * survive their deleting the application, which is precisely what deleting it
 * is supposed to undo.
 *
 * A new installation therefore has a new identity and must prove access to the
 * mailbox again. That is the requirement, not a side effect: every install is
 * confirmed separately.
 */
class DeviceIdentity private constructor(val deviceId: String) {

    companion object {
        fun of(context: Context): DeviceIdentity {
            val prefs = context.getSharedPreferences("identity", Context.MODE_PRIVATE)
            val existing = prefs.getString(KEY_DEVICE, null)
            if (existing != null) return DeviceIdentity(existing)

            val fresh = UUID.randomUUID().toString()
            prefs.edit().putString(KEY_DEVICE, fresh).apply()
            return DeviceIdentity(fresh)
        }

        private const val KEY_DEVICE = "device_id"
    }
}
