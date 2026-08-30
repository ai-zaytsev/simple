package download.simplevpn.update

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import download.simplevpn.BuildConfig
import download.simplevpn.plan.ConfigSource
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch

/** Process-scoped update state, independent of Activity lifetime. */
object UpdateController {
    enum class Phase { IDLE, NEEDS_PERMISSION, DOWNLOADING, INSTALLING, FAILED }

    data class State(
        val verdict: AppUpdatePolicy.Verdict = AppUpdatePolicy.Verdict.Current,
        val phase: Phase = Phase.IDLE,
        val failure: UpdateFailure? = null,
        val optionalDismissed: Boolean = false,
    ) {
        val required: Boolean get() = verdict is AppUpdatePolicy.Verdict.Required
        val visible: Boolean get() = required ||
            (verdict is AppUpdatePolicy.Verdict.Optional && !optionalDismissed)
        val busy: Boolean get() = phase == Phase.DOWNLOADING || phase == Phase.INSTALLING
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val _state = MutableStateFlow(State())
    val state: StateFlow<State> = _state.asStateFlow()
    private var dismissedVersionCode: Int? = null

    fun refresh(context: Context) {
        val application = context.applicationContext
        scope.launch {
            val config = ConfigSource(application).current()
            val verdict = config?.update?.verdict(
                BuildConfig.VERSION_CODE,
                BuildConfig.UPDATE_CHANNEL,
            ) ?: return@launch
            val latest = when (verdict) {
                is AppUpdatePolicy.Verdict.Optional -> verdict.policy.latestVersionCode
                is AppUpdatePolicy.Verdict.Required -> verdict.policy.latestVersionCode
                AppUpdatePolicy.Verdict.Current -> null
            }
            val pendingFailure = consumeInstallerFailure(application)
            val old = _state.value
            val active = old.phase == Phase.DOWNLOADING || old.phase == Phase.INSTALLING
            _state.value = State(
                verdict = verdict,
                phase = when {
                    pendingFailure != null -> Phase.FAILED
                    active -> old.phase
                    else -> Phase.IDLE
                },
                failure = pendingFailure ?: if (active) old.failure else null,
                optionalDismissed = latest != null && latest == dismissedVersionCode,
            )
        }
    }

    fun dismissOptional() {
        val current = _state.value
        val optional = current.verdict as? AppUpdatePolicy.Verdict.Optional ?: return
        dismissedVersionCode = optional.policy.latestVersionCode
        _state.value = current.copy(optionalDismissed = true, phase = Phase.IDLE, failure = null)
    }

    fun begin(context: Context) {
        val current = _state.value
        val pair = when (val verdict = current.verdict) {
            is AppUpdatePolicy.Verdict.Optional -> verdict.policy to verdict.artifact
            is AppUpdatePolicy.Verdict.Required -> verdict.policy to verdict.artifact
            AppUpdatePolicy.Verdict.Current -> return
        }
        val artifact = pair.second
        if (BuildConfig.UPDATE_CHANNEL != AppUpdatePolicy.DIRECT_APK || artifact == null) {
            _state.value = current.copy(phase = Phase.FAILED, failure = UpdateFailure.UNAVAILABLE)
            return
        }

        val application = context.applicationContext
        if (!DirectApkUpdater.mayInstall(application)) {
            _state.value = current.copy(phase = Phase.NEEDS_PERMISSION, failure = null)
            val settings = Intent(
                Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                Uri.parse("package:${context.packageName}"),
            )
            if (context !is android.app.Activity) settings.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            if (runCatching { context.startActivity(settings) }.isFailure) {
                _state.value = current.copy(phase = Phase.FAILED, failure = UpdateFailure.PERMISSION)
            }
            return
        }

        _state.value = current.copy(phase = Phase.DOWNLOADING, failure = null)
        scope.launch {
            when (val result = DirectApkUpdater.install(application, artifact, pair.first.latestVersionCode)) {
                DirectApkUpdater.Result.Committed -> {
                    _state.value = _state.value.copy(phase = Phase.INSTALLING, failure = null)
                }
                is DirectApkUpdater.Result.Failed -> {
                    _state.value = _state.value.copy(phase = Phase.FAILED, failure = result.failure)
                }
            }
        }
    }

    internal fun installerFailed(context: Context) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
            .edit().putBoolean(KEY_INSTALLER_FAILED, true).apply()
        _state.value = _state.value.copy(phase = Phase.FAILED, failure = UpdateFailure.INSTALLER)
        refresh(context)
    }

    private fun consumeInstallerFailure(context: Context): UpdateFailure? {
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        if (!prefs.getBoolean(KEY_INSTALLER_FAILED, false)) return null
        prefs.edit().remove(KEY_INSTALLER_FAILED).apply()
        return UpdateFailure.INSTALLER
    }

    private const val PREFS = "app_update"
    private const val KEY_INSTALLER_FAILED = "installer_failed"
}
