package download.simplevpn.vpn

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import download.simplevpn.MainActivity
import download.simplevpn.R
import download.simplevpn.config.RoutingPolicy
import download.simplevpn.config.XrayConfigBuilder
import download.simplevpn.core.BridgeDiagnostics
import download.simplevpn.core.EngineStartResult
import download.simplevpn.core.LibXrayEngine
import download.simplevpn.core.SessionLog
import download.simplevpn.core.TunBridge
import download.simplevpn.core.XrayEngine
import download.simplevpn.net.NetworkMonitor
import download.simplevpn.config.ConnectionProfile
import download.simplevpn.plan.AlreadyTunnelled
import download.simplevpn.plan.ConfigSource
import download.simplevpn.plan.EndpointChoice
import download.simplevpn.auth.AccountStore
import download.simplevpn.plan.PlanSource
import download.simplevpn.plan.PlanStore
import download.simplevpn.plan.ServiceConfig
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Owns the tunnel for the lifetime of the connection.
 *
 * The tunnel lives here and not in the Activity, so closing the screen leaves
 * it running. Everything the service does is wrapped so that a transport
 * failure ends the connection with a message instead of ending the process: an
 * engine that cannot start is an expected outcome, not a crash.
 */
class SimpleVpnService : VpnService() {

    private val engine: XrayEngine = LibXrayEngine(this)
    private val bridge = TunBridge()
    private var tunnel: ParcelFileDescriptor? = null

    /**
     * Once the descriptor is handed to the bridge it is no longer ours to
     * close: closing it on both sides closes it twice.
     */
    private var tunnelHandedOver = false
    private var networkMonitor: NetworkMonitor? = null
    private val starting = AtomicBoolean(false)

    /** Runs the delayed restart; see scheduleEngineRestart. */
    private val restartHandler = Handler(Looper.getMainLooper())
    private val restartEngine = Runnable { restartWhenNodeAnswers() }

    /** Probing opens a socket and waits, which the main thread must not do. */
    private val probes = Executors.newSingleThreadExecutor()

    /** Establishing now includes a network call, which the main thread forbids. */
    private val starts = Executors.newSingleThreadExecutor()

    /** Where the endpoint comes from now that the build does not carry one. */
    private val planSource by lazy { PlanSource(this) }

    /** Whether this installation is allowed to run at all. */
    private val configSource by lazy { ConfigSource(this) }

    /** Whether an endpoint can carry a tunnel, asked outside the tunnel. */
    private val health by lazy { EndpointHealth { socket -> protect(socket) } }
    private var probeAttempts = 0

    // Set while a failed plan is being replaced, so the session log keeps the
    // reason the previous one was abandoned.
    private var rebuilding = false

    /**
     * The endpoints of the plan in use and which one is live.
     *
     * Held rather than looked up each time, because failover means moving
     * along this list, and a list re-read from a stored plan would keep
     * starting again at a node just found to be dead.
     */
    private var endpoints: List<ConnectionProfile> = emptyList()
    private var currentIndex = 0
    private var currentEndpoint: ConnectionProfile? = null
    private var failures = EndpointChoice.Failures(DEFAULT_FAILOVER_AFTER)

    // Where traffic goes, as the last plan said. Held because the engine is
    // rebuilt on a network change and on a failover, and it must be rebuilt
    // with the same rules rather than with whatever the build was born with.
    private var routing = RoutingPolicy.UNTIL_A_PLAN_ARRIVES

    /** Watches the node in use and the switch that can stop everything. */
    private val watchHandler = Handler(Looper.getMainLooper())
    private val watchEndpoint = Runnable { checkEndpoint() }
    private val watchConfig = Runnable { checkConfig() }
    private var probeIntervalMs = DEFAULT_PROBE_INTERVAL_MS
    private var connectTimeoutMs = DEFAULT_CONNECT_TIMEOUT_MS

    // How long until the next question about the service. Set from the answer
    // to the last one, so the server owns the cadence and the client has no
    // number of its own to disagree with it.
    private var configRefreshMs = DEFAULT_CONFIG_REFRESH_MS

    override fun onCreate() {
        super.onCreate()
        createNotificationChannel()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_START -> {
                // The notification goes up here, on the calling thread, because
                // the system requires it promptly and killing the service for
                // being slow would look like a crash.
                startForeground(NOTIFICATION_ID, buildNotification(getString(R.string.status_connecting)))
                VpnController.update(VpnConnectionState.Connecting)

                // Everything after it moves off this thread: establishing now
                // includes asking the Control Plane where to connect, and a
                // network call on the main thread is an immediate crash.
                starts.execute { handleStart() }
            }

            // Somebody is looking at the application right now. Whatever the
            // timer would have asked in a few minutes, ask it now.
            ACTION_RECHECK -> if (engine.isRunning) {
                watchHandler.removeCallbacks(watchConfig)
                watchHandler.post(watchConfig)
            }

            ACTION_STOP -> handleStop(VpnConnectionState.Disconnected)
            else -> {
                // Restarted by the system with a null intent. Nothing is known
                // about the previous session, so the safe action is to stop
                // rather than to establish a tunnel the user did not ask for.
                Log.i(TAG, "restarted without an action, stopping")
                handleStop(VpnConnectionState.Disconnected)
            }
        }
        // START_NOT_STICKY: the system must not silently recreate a tunnel.
        // A VPN that comes back without the user asking is a surprise, and
        // surprises about traffic routing are not acceptable.
        return START_NOT_STICKY
    }

    private fun handleStart() {
        if (!starting.compareAndSet(false, true)) {
            Log.i(TAG, "start already in progress")
            return
        }

        // Cleared here and not later: everything below belongs to this attempt,
        // and a log holding two of them is worse than none.
        //
        // Except when this attempt follows a plan that was just abandoned.
        // Then the reason it was abandoned is the most useful thing in the
        // file, and wiping it would leave a log showing a connection that
        // works and no trace of what it replaced.
        if (!rebuilding) SessionLog.reset(this)

        try {
            // Asked before anything is established. This is the one answer that
            // can stop the whole product, and asking after the tunnel is up
            // would mean the switch takes effect only for the next person to
            // press the button.
            val config = configSource.current()
            if (config != null) {
                configRefreshMs = config.refreshAfterSeconds * 1000L
                SessionLog.record(this, "configuration: service running")
                if (!allowedToRun(config)) return
            } else {
                SessionLog.record(this, "configuration unavailable, using what is stored")
            }

            // Where to connect is asked for, not compiled in. This is the
            // whole point of the stage: the endpoint can change on the server
            // without anybody installing anything.
            val planResult = planSource.currentProfile()
            if (planResult is PlanSource.Result.Revoked) {
                signOutAndStop()
                return
            }
            if (planResult is PlanSource.Result.Missing) {
                SessionLog.record(this, "no endpoint: ${planResult.reason}")
                failAndStop(planResult.reason)
                return
            }
            val available = planResult as PlanSource.Result.Available
            adoptPlan(available)

            // A router running this VPN is invisible from the phone, which sees
            // ordinary Wi-Fi. What gives it away is the address we are seen
            // from: if it is one of our own nodes, the network already goes
            // through us and a second tunnel would nest inside the first.
            when (val already = AlreadyTunnelled.decide(planSource.seenFrom(), endpoints)) {
                is AlreadyTunnelled.Verdict.ThroughOurNode -> {
                    SessionLog.record(this, "network already runs through node ${already.alias}, not building a second tunnel")
                    failAndStop(getString(R.string.error_already_tunnelled))
                    return
                }

                is AlreadyTunnelled.Verdict.Unknown ->
                    SessionLog.record(this, "could not tell whether the network is already tunnelled, continuing")

                is AlreadyTunnelled.Verdict.NotTunnelled -> Unit
            }

            // The first endpoint that answers, in the order the server chose.
            // A reserve nobody ever tries is a reserve that does not exist.
            val choice = EndpointChoice.choose(endpoints) { reachable(it) }
            if (choice == null) {
                failAndStop(getString(R.string.error_unexpected))
                return
            }
            currentIndex = choice.index
            currentEndpoint = choice.endpoint
            val profile = choice.endpoint

            SessionLog.record(this, "endpoint from ${available.source}")
            if (!choice.probed) {
                // Worth saying plainly. Every endpoint failed a plain TCP
                // connect and the primary is being used anyway, which is a
                // guess rather than a choice.
                SessionLog.record(this, "no endpoint answered a probe, using the primary")
            } else if (choice.index > 0) {
                SessionLog.record(this, "primary did not answer, using reserve ${choice.index}")
            }
            SessionLog.record(
                this,
                "endpoint ${profile.host}:${profile.port} " +
                    "transport ${profile.transport::class.simpleName}",
            )

            // From the plan, not from the build. A route that turns out wrong is
            // then an operator changing a row, not a release.
            val policy = routing

            val descriptor = TunConfigurator(this).establish(policy)
            if (descriptor == null) {
                SessionLog.record(this, "interface not established")
                failAndStop(getString(R.string.error_tun_not_established))
                return
            }
            tunnel = descriptor
            SessionLog.record(this, "interface established, mtu ${TunConfigurator.MTU}")

            val configJson = XrayConfigBuilder.build(profile, policy, SessionLog.engineFile(this).absolutePath)

            when (val result = engine.start(configJson, descriptor.fd)) {
                is EngineStartResult.Started -> SessionLog.record(this, "engine started")

                is EngineStartResult.Unavailable -> {
                    SessionLog.record(this, "engine unavailable: ${result.reason}")
                    failAndStop(result.reason)
                    return
                }

                is EngineStartResult.Failed -> {
                    Log.w(TAG, "engine failed to start", result.cause)
                    SessionLog.record(this, "engine failed: ${result.reason}")
                    failAndStop(result.reason)
                    return
                }
            }

            // The engine only listens on loopback. Until the bridge is running,
            // not a single packet from the device reaches it, so the tunnel is
            // not established until this succeeds.
            val rawFd = descriptor.detachFd()
            tunnelHandedOver = true

            when (val bridged = bridge.start(rawFd, TunConfigurator.MTU, XrayConfigBuilder.SOCKS_PORT)) {
                is TunBridge.Result.Started -> {
                    SessionLog.record(this, "bridge started, socks ${XrayConfigBuilder.SOCKS_PORT}")
                    startNetworkMonitor()
                    startWatching()
                    VpnController.update(VpnConnectionState.Connected(System.currentTimeMillis()))
                    updateNotification(getString(R.string.status_connected))
                    SessionLog.record(this, "connected")

                    // "Connected" has never meant "working". Everything above
                    // succeeds when the node refuses the credential, when a
                    // routing rule sends everything nowhere, and when the plan
                    // names an endpoint that has been withdrawn - all three
                    // have happened here, and each looked like success.
                    probes.execute { proveOrRollBack(planSource.sourceInUse()) }
                }

                is TunBridge.Result.Unavailable -> {
                    SessionLog.record(this, "bridge unavailable: ${bridged.reason}")
                    failAndStop(bridged.reason)
                }

                is TunBridge.Result.Failed -> {
                    SessionLog.record(this, "bridge failed: ${bridged.reason}")
                    failAndStop(bridged.reason)
                }
            }
        } catch (t: Throwable) {
            SessionLog.record(this, "unexpected failure while starting: ${t.message}")
            // Deliberately broad. Anything thrown while establishing must end
            // as a reported failure: a crash here would take the process down
            // and leave the system VPN slot in an unclear state.
            Log.e(TAG, "unexpected failure while starting", t)
            failAndStop(getString(R.string.error_unexpected))
        } finally {
            starting.set(false)
        }
    }

    /**
     * Confirms the tunnel carries traffic, and rolls back when it does not.
     *
     * This is the whole of the stage. A mistake in settings must not break the
     * product for everybody at once: a plan is a proposal until it has carried
     * something, and one that cannot is abandoned for the last plan that could.
     * The person does nothing; they see a reconnection.
     *
     * Bounded, because the fallback can fail too. Without a limit a device with
     * two bad plans would rebuild the tunnel for ever, which is worse than
     * saying plainly that nothing works.
     */
    private fun proveOrRollBack(source: PlanStore.Source) {
        // Deliberately broad, and it is not defensive habit. This runs on a
        // background thread, and an exception thrown here takes the process
        // down with it: the tunnel disappears, the session log stops
        // mid-sentence, and the person is left pressing the button again with
        // nothing to explain why. That is what a live test showed happening.
        try {
            proveOrRollBackOrThrow(source)
        } catch (t: Throwable) {
            Log.e(TAG, "failed while proving the tunnel", t)
            SessionLog.record(this, "unexpected failure while proving the tunnel: ${t.message}")
            failAndStop(getString(R.string.error_unexpected))
        }
    }

    private fun proveOrRollBackOrThrow(source: PlanStore.Source) {
        if (!engine.isRunning) return

        if (TunnelProof.carriesTraffic(connectTimeoutMs)) {
            SessionLog.record(this, "tunnel carries traffic")
            planSource.proved(source)
            rebuilding = false
            return
        }

        val seq = planSource.candidateSeq()
        SessionLog.record(this, "nothing came back through the tunnel, plan $seq")
        planSource.failed(source)

        // Told to the server as well as acted on. Rolling back here is half of
        // not breaking the product for everybody; the other half is somebody
        // finding out that a plan is failing in the field, or the next person
        // to install gets the same one.
        starts.execute { planSource.reportFailure(seq, "no traffic through the tunnel") }

        // When the plan that just failed was already the fallback, there is
        // nothing older to fall back to and rebuilding would try the same two
        // plans for ever.
        //
        // Judged from what the store remembers rather than from a counter in
        // this object, because this object does not always survive: a live test
        // showed the process ending between attempts, and an in-memory count
        // silently started again from zero each time - a bound that bounded
        // nothing.
        if (source == PlanStore.Source.KNOWN_GOOD) {
            SessionLog.record(this, "neither the newest plan nor the last good one works")
            failAndStop(getString(R.string.error_no_working_plan))
            return
        }

        SessionLog.record(this, "trying another plan")
        VpnController.update(VpnConnectionState.Reconnecting)
        updateNotification(getString(R.string.status_reconnecting))

        // A full rebuild rather than an engine restart: a different plan can
        // name different applications to keep out of the tunnel, and that is
        // decided when the interface is built.
        //
        // Both halves run on the same thread that establishes, so a rebuild
        // cannot overlap with the restart that a network change schedules.
        // Overlapping them was what left an interface half torn down while
        // another thread was building one.
        rebuilding = true
        starts.execute {
            teardown()
            handleStart()
        }
    }

    /**
     * Takes the numbers from the plan instead of inventing them.
     *
     * The server decides how long to wait for a node, how many failures mean
     * it is gone, and how often to look. A client with its own opinions about
     * those is a client whose behaviour cannot be changed without an update.
     */
    private fun adoptPlan(available: PlanSource.Result.Available) {
        endpoints = available.plan.endpoints
        routing = available.plan.routing
        connectTimeoutMs = available.plan.connectTimeoutMs
        probeIntervalMs = available.plan.probeIntervalSeconds * 1000L
        failures = EndpointChoice.Failures(available.plan.failoverAfterFailures)
    }

    /**
     * Stops this installation when the server says the service is stopped.
     *
     * @return false when it may not run, having already stopped the service
     */
    private fun allowedToRun(config: ServiceConfig): Boolean {
        return when (val verdict = config.verdict(APP_VERSION)) {
            is ServiceConfig.Verdict.Allowed -> true

            is ServiceConfig.Verdict.Stopped -> {
                val message = when (verdict.reason) {
                    ServiceConfig.Stop.KILL_SWITCH -> getString(R.string.error_service_stopped)
                    ServiceConfig.Stop.TOO_OLD -> getString(R.string.error_app_too_old)
                }
                SessionLog.record(this, "refused by configuration: ${verdict.reason}")
                failAndStop(message)
                false
            }
        }
    }

    /**
     * Watches the node in use, and the switch that can stop everything.
     *
     * Two separate rhythms because they answer different questions at
     * different costs. Probing the node is a TCP connect over the network the
     * phone is already using; asking the server is a request that must not
     * happen every minute for every device.
     */
    private fun startWatching() {
        watchHandler.removeCallbacks(watchEndpoint)
        watchHandler.removeCallbacks(watchConfig)
        failures.succeeded()
        watchHandler.postDelayed(watchEndpoint, probeIntervalMs)
        watchHandler.postDelayed(watchConfig, configRefreshMs)
    }

    /**
     * Moves to the next endpoint when the one in use stops answering.
     *
     * This is the failure the reserves exist for and the one nothing detected
     * before: a node that dies while somebody is connected. A network change
     * announces itself; a node going away does not, and without this the
     * connection sat there carrying nothing until the person noticed.
     */
    private fun checkEndpoint() {
        if (!engine.isRunning) return

        probes.execute {
            val alive = currentEndpoint?.let { reachable(it) } ?: true

            watchHandler.post {
                if (!engine.isRunning) return@post

                if (alive) {
                    failures.succeeded()
                } else if (failures.failed()) {
                    val next = EndpointChoice.next(endpoints, currentIndex)
                    if (next != null && endpoints.size > 1) {
                        SessionLog.record(
                            this,
                            "endpoint stopped answering ${failures.count} times, moving to ${next.host}:${next.port}",
                        )
                        currentIndex = (currentIndex + 1) % endpoints.size
                        currentEndpoint = next
                        failures.succeeded()
                        restartEngineOnCurrentEndpoint("failover")
                    } else {
                        // Nowhere to go. Saying so is better than moving from a
                        // node to itself and calling it a recovery.
                        SessionLog.record(this, "endpoint not answering and there is no reserve")
                    }
                }

                watchHandler.postDelayed(watchEndpoint, probeIntervalMs)
            }
        }
    }

    /**
     * Ends the session because this installation is no longer recognised.
     *
     * Everything about being signed in goes, because none of it works any more:
     * the secret is not one the server knows, and the stored plan names a
     * credential no node will accept. Leaving either behind would produce an
     * application that looks signed in and fails at every turn.
     *
     * The reason is written down rather than shown from here, because by the
     * time anybody reads it this screen is gone and the sign-in screen has
     * taken its place. It is shown there.
     */
    private fun signOutAndStop() {
        SessionLog.record(this, "this installation is no longer recognised, signing out")
        AccountStore(this).signedOutElsewhere()
        PlanStore(this).clear()
        failAndStop(getString(R.string.error_signed_in_elsewhere))
    }

    /**
     * Asks whether the service has been stopped since this connection began.
     *
     * The next question is scheduled by the answer to this one, and by nothing
     * else. There used to be a second interval here: the service woke every
     * five minutes, found a stored document it considered fresh for fifteen,
     * and asked nobody. The switch worked and took three times as long as
     * anybody had been told, which is the kind of wrong that is only ever
     * discovered by somebody waiting.
     */
    private fun checkConfig() {
        if (!engine.isRunning) return

        starts.execute {
            // Two questions on one rhythm: is the service running, and is
            // this device still one the server knows. They are asked together
            // because they are the two ways a running connection can become
            // one that should not continue.
            val standing = planSource.standing()
            val config = configSource.current()

            watchHandler.post {
                if (!engine.isRunning) return@post
                if (standing == PlanSource.Standing.REVOKED) {
                    signOutAndStop()
                    return@post
                }
                if (config != null) {
                    configRefreshMs = config.refreshAfterSeconds * 1000L
                    SessionLog.record(
                        this,
                        "configuration checked: running, asking again in ${config.refreshAfterSeconds}s",
                    )
                    if (!allowedToRun(config)) return@post
                } else {
                    SessionLog.record(this, "configuration could not be read, continuing on what is stored")
                }
                watchHandler.postDelayed(watchConfig, configRefreshMs)
            }
        }
    }

    /**
     * Waits for the network to settle before restarting anything.
     *
     * A restart costs every connection in flight. On Wi-Fi that is rare enough
     * not to matter; a mobile network changes constantly - moving between
     * cells, losing and regaining Wi-Fi - and a device reported two changes in
     * three and a half minutes, each of which dropped ninety-five live
     * connections. Some of those changes are not real: the system announces a
     * new network while the one in use is still there.
     *
     * Each change pushes the restart further out, so a burst of them costs one
     * restart instead of one each. What it cannot avoid is the restart itself
     * when the change is real: sockets opened over a network that is gone stay
     * open and deliver nothing.
     */
    private fun scheduleEngineRestart() {
        restartHandler.removeCallbacks(restartEngine)
        probeAttempts = 0
        restartHandler.postDelayed(restartEngine, NETWORK_SETTLE_MS)
    }

    /**
     * Restarts only once the node answers, and not merely once the system says
     * the network changed.
     *
     * A phone announces a new network before that network can carry anything.
     * Restarting into that window is worse than waiting: a device log showed
     * 549 failed dials to the node, 501 of them inside a single minute
     * following a switch to mobile, each one a retry against a network that was
     * not ready. The tunnel recovered on its own afterwards, so nothing was
     * broken - it was a minute of a phone talking to itself.
     *
     * A short connection to the node is cheap and answers the only question
     * that matters. Until it succeeds the restart is postponed, and the state
     * says reconnecting rather than claiming a connection that is not carrying
     * anything.
     *
     * After enough failed attempts the restart happens regardless: a node that
     * cannot be reached at all is a different failure, and the engine reporting
     * it is more useful than this quietly waiting forever.
     */
    private fun restartWhenNodeAnswers() {
        probes.execute {
            val reachable = probeNode()

            restartHandler.post {
                when {
                    reachable -> {
                        SessionLog.record(this, "node answers after $probeAttempts probe(s), restarting")
                        restartEngineForNewNetwork()
                    }

                    probeAttempts >= MAX_PROBE_ATTEMPTS -> {
                        SessionLog.record(this, "node did not answer in $probeAttempts probes, restarting anyway")
                        restartEngineForNewNetwork()
                    }

                    else -> {
                        if (probeAttempts == 0) {
                            VpnController.update(VpnConnectionState.Reconnecting)
                            updateNotification(getString(R.string.status_reconnecting))
                        }
                        probeAttempts++
                        val wait = minOf(PROBE_BACKOFF_MS * probeAttempts, PROBE_BACKOFF_CAP_MS)
                        restartHandler.postDelayed(restartEngine, wait)
                    }
                }
            }
        }
    }

    /**
     * One short connection to the node, outside the tunnel.
     *
     * Protected explicitly. The application already excludes itself from its
     * own tunnel, so this would escape anyway, but a probe that measured the
     * inside of the tunnel would answer the wrong question entirely.
     */
    private fun probeNode(): Boolean {
        val profile = currentEndpoint
            // Nothing to probe against. Restarting is then the only option that
            // can produce a message the user can act on.
            ?: return true
        return reachable(profile)
    }

    /**
     * @return whether this endpoint can carry a tunnel right now.
     *
     * Not merely whether the port answers. Port 443 belongs to Nginx, which
     * serves an ordinary website and answers whether or not the engine behind
     * it is alive; a node whose Xray had died would have passed that check
     * while carrying nothing, and failover would never have fired.
     */
    private fun reachable(profile: ConnectionProfile): Boolean =
        health.check(profile, connectTimeoutMs)

    private fun restartEngineForNewNetwork() =
        restartEngineOnCurrentEndpoint("underlying network changed")

    /**
     * Rebuilds the engine against whichever endpoint is current.
     *
     * Used by both reasons to restart, because they need exactly the same
     * thing done: a network change keeps the endpoint and replaces the sockets;
     * a failover replaces the endpoint. Two copies of this would be two places
     * for the retry rule below to be got wrong.
     */
    private fun restartEngineOnCurrentEndpoint(reason: String) {
        if (!engine.isRunning) return
        SessionLog.record(this, "$reason, restarting the engine")
        VpnController.update(VpnConnectionState.Reconnecting)
        updateNotification(getString(R.string.status_reconnecting))

        try {
            // Only the engine restarts. The bridge already owns the interface
            // and talks to loopback, so the address it forwards to does not
            // change when the underlying network does. Rebuilding the interface
            // here would drop the tunnel the user is currently using.
            engine.stop()

            // The endpoint in hand, not a fresh request. A network that has
            // just changed is the worst moment to depend on reaching a server,
            // and after a failover the answer is already decided.
            // Nothing to restart. This happens when a scheduled restart fires
            // while the tunnel is being rebuilt on another plan, and it is not a
            // failure: the rebuild will establish an interface of its own in a
            // moment. Ending the session here would take the tunnel away from
            // somebody whose connection was about to be restored.
            val profile = currentEndpoint ?: run {
                SessionLog.record(this, "restart arrived with no endpoint in hand, ignoring")
                return
            }

            val configJson = XrayConfigBuilder.build(
                profile,
                routing,
                SessionLog.engineFile(this).absolutePath,
            )
            when (val result = engine.start(configJson, TUN_FD_OWNED_BY_BRIDGE)) {
                is EngineStartResult.Started -> {
                    VpnController.update(VpnConnectionState.Connected(System.currentTimeMillis()))
                    updateNotification(getString(R.string.status_connected))
                }

                is EngineStartResult.Unavailable -> failAndStop(result.reason)

                is EngineStartResult.Failed -> {
                    // One retry, because the common failure here is a start
                    // that arrived before the stop it follows had finished and
                    // was refused as already running. Tearing the tunnel down
                    // for that turns a recoverable moment into a dropped
                    // connection the user has to fix by hand.
                    Log.w(TAG, "restart refused, stopping the engine and retrying: ${result.reason}")
                    engine.stop()

                    when (val retry = engine.start(configJson, TUN_FD_OWNED_BY_BRIDGE)) {
                        is EngineStartResult.Started -> {
                            VpnController.update(VpnConnectionState.Connected(System.currentTimeMillis()))
                            updateNotification(getString(R.string.status_connected))
                        }

                        is EngineStartResult.Unavailable -> failAndStop(retry.reason)
                        is EngineStartResult.Failed -> failAndStop(retry.reason)
                    }
                }
            }
        } catch (t: Throwable) {
            Log.e(TAG, "failed to restart after network change", t)
            failAndStop(getString(R.string.error_unexpected))
        }
    }

    private fun startNetworkMonitor() {
        networkMonitor?.stop()
        networkMonitor = NetworkMonitor(
            context = this,
            onUnderlyingNetworkChanged = { scheduleEngineRestart() },
            onNetworkLost = {
                // Not a failure: the device may be between networks. The state
                // says reconnecting, and the next available network triggers a
                // restart.
                VpnController.update(VpnConnectionState.Reconnecting)
                updateNotification(getString(R.string.status_reconnecting))
            },
        ).also { it.start() }
    }

    private fun failAndStop(reason: String) {
        rebuilding = false
        SessionLog.record(this, "stopping after failure: " + reason)
        VpnController.update(VpnConnectionState.Failed(reason))
        teardown()
        stopSelf()
    }

    private fun handleStop(finalState: VpnConnectionState) {
        rebuilding = false
        SessionLog.record(this, "stop requested")
        VpnController.update(VpnConnectionState.Disconnecting)
        teardown()
        VpnController.update(finalState)
        stopSelf()
    }

    private fun teardown() {
        // A restart that fires after teardown would rebuild an engine nobody
        // asked for, over an interface that is already gone.
        restartHandler.removeCallbacks(restartEngine)
        watchHandler.removeCallbacks(watchEndpoint)
        watchHandler.removeCallbacks(watchConfig)
        probeAttempts = 0
        currentEndpoint = null
        SessionLog.record(this, "teardown, bridge counters: " + BridgeDiagnostics.snapshot())
        networkMonitor?.stop()
        networkMonitor = null

        // Bridge first: it holds the interface, and stopping the engine
        // underneath it would leave packets arriving with nowhere to go.
        bridge.stop()

        try {
            engine.stop()
        } catch (t: Throwable) {
            Log.w(TAG, "engine stop failed", t)
        }

        try {
            // Only when the bridge never took it. After hand-over the bridge
            // owns the descriptor and has already released it.
            if (!tunnelHandedOver) {
                tunnel?.close()
            }
        } catch (t: Throwable) {
            Log.w(TAG, "closing tunnel failed", t)
        } finally {
            tunnel = null
            tunnelHandedOver = false
        }

        stopForeground(STOP_FOREGROUND_REMOVE)
    }

    /** The user revoked consent, or another VPN application took over. */
    override fun onRevoke() {
        Log.i(TAG, "consent revoked")
        SessionLog.record(this, "consent revoked or another VPN took over")
        teardown()
        VpnController.update(VpnConnectionState.Disconnected)
        stopSelf()
        super.onRevoke()
    }

    override fun onDestroy() {
        teardown()
        if (VpnController.state.value.isActive) {
            VpnController.update(VpnConnectionState.Disconnected)
        }
        super.onDestroy()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val channel = NotificationChannel(
            CHANNEL_ID,
            getString(R.string.notification_channel_name),
            NotificationManager.IMPORTANCE_LOW,
        )
        val manager = getSystemService(NotificationManager::class.java)
        manager?.createNotificationChannel(channel)
    }

    private fun buildNotification(text: String): Notification {
        val open = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE,
        )

        // NotificationCompat rather than Notification.Builder: the latter needs
        // a channel from API 26 and the minimum supported level here is 24.
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setSmallIcon(R.drawable.ic_launcher)
            .setContentIntent(open)
            .setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(NotificationManager::class.java) ?: return
        try {
            manager.notify(NOTIFICATION_ID, buildNotification(text))
        } catch (t: Throwable) {
            // Notification permission may be absent. The tunnel does not depend
            // on the notification being visible, so this never fails a start.
            Log.w(TAG, "could not update notification", t)
        }
    }

    companion object {
        const val ACTION_START = "download.simplevpn.action.START"
        const val ACTION_STOP = "download.simplevpn.action.STOP"
        const val ACTION_RECHECK = "download.simplevpn.action.RECHECK"

        private const val TAG = "SimpleVpnService"

        /**
         * The engine never touches the interface: the bridge owns it. The
         * parameter stays on the interface because a future transport may need
         * it, and passing a descriptor the engine must not use would be worse
         * than passing none.
         */
        private const val TUN_FD_OWNED_BY_BRIDGE = 0
        /**
         * How long the network is given to settle before a restart.
         *
         * Long enough to swallow a burst of announcements, which arrive within
         * a second or two of each other, and short enough that traffic is not
         * left going through dead sockets for noticeably longer than before.
         */
        private const val NETWORK_SETTLE_MS = 3_000L

        /** How long one probe waits before calling the node unreachable. */
        private const val PROBE_TIMEOUT_MS = 4_000

        /** Grows with each attempt, so a long outage is not a tight loop. */
        private const val PROBE_BACKOFF_MS = 2_000L
        private const val PROBE_BACKOFF_CAP_MS = 8_000L

        /**
         * After this many, restart regardless. A node that cannot be reached
         * at all is a different failure, and the engine reporting it beats
         * waiting in silence.
         */
        private const val MAX_PROBE_ATTEMPTS = 10


        /**
         * What this build calls itself when the server asks how old it is.
         *
         * A plain integer rather than a version name: the server compares it,
         * and a comparison of names is a comparison that eventually gets an
         * unexpected name and does the wrong thing quietly.
         */
        private const val APP_VERSION = 1

        /** Used until a plan says otherwise. */
        private const val DEFAULT_PROBE_INTERVAL_MS = 60_000L
        private const val DEFAULT_CONNECT_TIMEOUT_MS = 8_000
        private const val DEFAULT_FAILOVER_AFTER = 2

        /**
         * How often a running connection asks whether the service has been
         * stopped. Five minutes: often enough that a switch thrown now reaches
         * a connected phone soon, rarely enough that it is not a request a
         * minute from every device.
         */
        /** Used only until the first document arrives and says otherwise. */
        private const val DEFAULT_CONFIG_REFRESH_MS = 5 * 60 * 1000L
        private const val CHANNEL_ID = "vpn_status"
        private const val NOTIFICATION_ID = 1
    }
}
