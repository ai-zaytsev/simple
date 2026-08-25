package download.simplevpn.plan

import org.json.JSONObject

/**
 * What the server says about the service as a whole, rather than about one
 * device.
 *
 * Two things live here and both are ways of saying stop. The kill switch stops
 * everybody at once; the minimum version stops builds that are too old to be
 * safe to keep running. Neither can be expressed in a plan, because a plan is
 * per device and these are not.
 */
data class ServiceConfig(
    val seq: Long,
    val killSwitch: Boolean,
    val killMessageKey: String,
    val minSupportedAppVersion: Int,
    val refreshAfterSeconds: Int,
) {

    /**
     * Whether this build may connect.
     *
     * A pure function of the document and the build, deliberately: this is the
     * one decision that can take the whole product off the air, and it must be
     * possible to state it, read it and test it without a device, a network or
     * a running service.
     *
     * The kill switch is checked first. When both apply, the honest thing to
     * say is the one that is true of everybody rather than the one that blames
     * the person's build.
     */
    fun verdict(appVersion: Int): Verdict = when {
        killSwitch -> Verdict.Stopped(Stop.KILL_SWITCH)
        appVersion < minSupportedAppVersion -> Verdict.Stopped(Stop.TOO_OLD)
        else -> Verdict.Allowed
    }

    sealed interface Verdict {
        data object Allowed : Verdict
        data class Stopped(val reason: Stop) : Verdict
    }

    enum class Stop { KILL_SWITCH, TOO_OLD }

    companion object {

        fun parse(payload: JSONObject): ServiceConfig? {
            return try {
                if (payload.getInt("v") != SUPPORTED_VERSION) return null

                val kill = payload.optJSONObject("kill_switch")
                ServiceConfig(
                    seq = payload.getLong("seq"),
                    // Absent means off. A document that does not mention the
                    // switch is a document from a version that did not have
                    // one, and inventing "on" from silence would take the
                    // service down on an upgrade.
                    killSwitch = kill?.optBoolean("enabled", false) ?: false,
                    killMessageKey = kill?.optString("message_key", "") ?: "",
                    minSupportedAppVersion = payload.optInt("min_supported_app_version", 1),
                    refreshAfterSeconds = payload.optInt("refresh_after_s", DEFAULT_REFRESH),
                )
            } catch (t: Throwable) {
                null
            }
        }

        /**
         * Whether a freshly received document may replace the one in use.
         *
         * Higher numbers only. Somebody who recorded a document from before the
         * switch was thrown could otherwise serve it back and turn the switch
         * off, which would make the switch worth nothing against exactly the
         * adversary it exists for.
         */
        fun supersedes(candidate: ServiceConfig, current: ServiceConfig?): Boolean =
            current == null || candidate.seq > current.seq

        private const val SUPPORTED_VERSION = 1
        private const val DEFAULT_REFRESH = 900
    }
}
