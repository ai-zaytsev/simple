package download.simplevpn.core

/**
 * Placeholder used while the libXray AAR binding is not yet written.
 *
 * It refuses to start and says why. It deliberately does not pretend to
 * succeed: a build that reports a working tunnel without one would satisfy the
 * UI and betray the user.
 *
 * Replaced by the real binding once the AAR public API has been inspected;
 * see the libxray-build workflow, which dumps that API.
 */
class PendingXrayEngine : XrayEngine {

    override val isRunning: Boolean = false

    override fun start(configJson: String, tunFd: Int): EngineStartResult =
        EngineStartResult.Unavailable(
            "Transport engine is not bundled in this build yet",
        )

    override fun stop() = Unit
}
