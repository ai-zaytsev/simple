package download.simplevpn.core

/**
 * Boundary between the application and the transport implementation.
 *
 * Nothing outside this package knows that the transport is Xray, that the
 * protocol is VLESS or that the camouflage is REALITY. That isolation is
 * invariant I-14 in docs/architecture/evolution.md: adding a second transport
 * later must not touch the session controller, the UI or telemetry.
 */
interface XrayEngine {

    val isRunning: Boolean

    /**
     * @param configJson complete engine configuration
     * @param tunFd file descriptor of the established TUN interface
     */
    fun start(configJson: String, tunFd: Int): EngineStartResult

    fun stop()
}

sealed interface EngineStartResult {

    data object Started : EngineStartResult

    /**
     * The engine is not present in this build. Distinct from [Failed] because
     * it is a build-time fact, not a runtime error, and the user-facing message
     * differs accordingly.
     */
    data class Unavailable(val reason: String) : EngineStartResult

    data class Failed(val reason: String, val cause: Throwable? = null) : EngineStartResult
}
