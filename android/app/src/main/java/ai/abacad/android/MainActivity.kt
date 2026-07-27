package ai.abacad.android

import android.app.Activity
import android.app.KeyguardManager
import android.content.ActivityNotFoundException
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Environment
import android.os.PowerManager
import android.provider.Settings
import android.text.format.DateFormat
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.ActivityResultLauncher
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import java.util.Date

/**
 * The abacad device agent's single screen, in Jetpack Compose + Material 3.
 *
 * It is the consent + awareness surface for the person whose phone is being
 * driven, built around one `ready` flag:
 *  - **Not ready** (disconnected, or the required Accessibility service is off):
 *    setup is the screen — server URL, Scan QR, Connect, and the capability
 *    checklist with a colored dot per row (tap to fix).
 *  - **Ready** (connected + accessibility on): the live surface leads — a
 *    "Controlling now" / "Connected" state, "screen being watched" / "recording"
 *    flags, a Pause / Disconnect pair, and the recent-actions tail; setup folds
 *    into a collapsed drawer that carries a "needs attention" badge.
 *
 * All device control happens over the network in [AbacadAccessibilityService];
 * this screen only observes [AbacadStatus] and issues connect / disconnect /
 * pause intents.
 */
class MainActivity : ComponentActivity() {

    private enum class Level { OK, WARN, BAD, INFO }

    /** One setup requirement: read its live state, and (optionally) fix it on tap. */
    private class Check(
        val title: String,
        val evaluate: () -> Pair<Level, String>,
        val onTap: (() -> Unit)?,
    )

    private data class RenderedCheck(val title: String, val level: Level, val detail: String, val onTap: (() -> Unit)?)

    private val prefs get() = getSharedPreferences(AbacadAccessibilityService.PREFS, Context.MODE_PRIVATE)

    private lateinit var scanLauncher: ActivityResultLauncher<Intent>
    private lateinit var notifPermLauncher: ActivityResultLauncher<String>

    // Compose observation: statusTick bumps on every AbacadStatus change; sysTick
    // bumps in onResume so the checklist re-reads live system state after the user
    // returns from a Settings page.
    private var statusTick by mutableIntStateOf(0)
    private var sysTick by mutableIntStateOf(0)
    private var url by mutableStateOf("")

    // Self-enrollment UI state. While unclaimed the screen shows deviceId +
    // claimCode; both come from the relay and the relay rotates the code, so we
    // only ever display what the last heartbeat handed us.
    private var deviceId by mutableStateOf("")
    private var claimCode by mutableStateOf("")
    private var relayUrl by mutableStateOf(Enrollment.DEFAULT_RELAY)
    private var enrolling by mutableStateOf(false)
    private var enrollError by mutableStateOf<String?>(null)

    /** Guards against two enrollment loops running after a rotation/resume. */
    @Volatile private var enrollThread: Thread? = null

    private val statusListener: () -> Unit = { runOnUiThread { statusTick++ } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        url = prefs.getString(AbacadAccessibilityService.KEY_SERVER_URL, "").orEmpty()
        deviceId = prefs.getString(AbacadAccessibilityService.KEY_DEVICE_ID, "").orEmpty()
        relayUrl = Enrollment.normalizeRelay(
            prefs.getString(AbacadAccessibilityService.KEY_RELAY_URL, "").orEmpty())

        scanLauncher = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { res ->
            if (res.resultCode != Activity.RESULT_OK) return@registerForActivityResult
            val scanned = res.data?.getStringExtra(ScanActivity.RESULT_TEXT)?.trim().orEmpty()
            if (scanned.isNotEmpty()) {
                url = scanned
                saveAndConnect(scanned)
                toast("Scanned. Connecting…")
            }
        }
        notifPermLauncher = registerForActivityResult(ActivityResultContracts.RequestPermission()) { }

        setContent {
            val dark = isSystemInDarkTheme()
            val palette = abacadColors(dark)
            MaterialTheme(colorScheme = abacadColorScheme(dark)) {
                // Read the ticks so recomposition follows status + resume.
                val st = statusTick
                val sy = sysTick
                val snap = remember(st) { snapshot() }
                val accessibilityOn = remember(sy) { isAccessibilityEnabled() }
                val checks = remember(sy) {
                    buildChecks().map { c -> val (lv, d) = c.evaluate(); RenderedCheck(c.title, lv, d, c.onTap) }
                }
                val ready = snap.state == AbacadStatus.State.CONNECTED && accessibilityOn

                Surface(modifier = Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
                    Column(
                        modifier = Modifier
                            .fillMaxSize()
                            .verticalScroll(rememberScrollState())
                            .padding(AbacadDim.spaceLg),
                    ) {
                        Text(
                            "abacad",
                            color = MaterialTheme.colorScheme.onBackground,
                            fontFamily = FontFamily.SansSerif,
                            fontWeight = FontWeight.Bold,
                            style = MaterialTheme.typography.titleLarge,
                        )
                        Spacer(Modifier.height(AbacadDim.spaceLg))

                        StateBanner(snap, palette, ready, onPause = ::togglePause, onDisconnect = ::disconnect)

                        if (ready) {
                            AwarenessFlags(snap, palette)
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            SectionLabel("Recent actions")
                            RecentActions(snap, palette)
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            SetupDrawer(checks, palette, startExpanded = false)
                        } else {
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            // Self-enrollment is the primary path: this phone shows
                            // its own ID and claim code, and nothing secret has to
                            // travel toward it. The QR/URL route stays behind
                            // "Other ways to connect" for browser-issued
                            // credentials and relays this app can't reach.
                            ClaimSection(
                                deviceId = deviceId,
                                claimCode = claimCode,
                                relay = relayUrl,
                                enrolling = enrolling,
                                error = enrollError,
                                palette = palette,
                                onRetry = ::startEnrollment,
                            )
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            ConnectSection(
                                url = url,
                                onUrl = { url = it },
                                onScan = { scanLauncher.launch(Intent(this@MainActivity, ScanActivity::class.java)) },
                                onConnect = { saveAndConnect(url); toast("Saved. Connecting…") },
                            )
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            SectionLabel("Setup")
                            Checklist(checks, palette)
                            Spacer(Modifier.height(AbacadDim.spaceLg))
                            SetupHelp(palette)
                        }
                    }
                }
            }
        }

        // Android 13+: the ongoing foreground-service notification is the user's
        // signal the device is connected — ask for POST_NOTIFICATIONS once up front.
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            notifPermLauncher.launch(android.Manifest.permission.POST_NOTIFICATIONS)
        }
    }

    override fun onResume() {
        super.onResume()
        AbacadStatus.addListener(statusListener)
        statusTick++
        sysTick++
        // Kick (or resume) enrollment whenever the setup screen is in front of
        // someone. No-ops when a URL is configured or a loop is already running.
        startEnrollment()
    }

    override fun onPause() {
        super.onPause()
        // Stop polling once nobody is looking at the code. The service holds the
        // connection after a claim; this loop only exists to drive the UI.
        enrollThread?.interrupt()
        enrollThread = null
        enrolling = false
    }

    override fun onPause() {
        super.onPause()
        AbacadStatus.removeListener(statusListener)
    }

    // ---- actions --------------------------------------------------------------

    private fun saveAndConnect(u: String) {
        val clean = u.trim()
        prefs.edit().putString(AbacadAccessibilityService.KEY_SERVER_URL, clean).apply()
        // Don't echo the URL — it carries the device token.
        sendBroadcast(Intent(AbacadAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
    }

    // ---- self-enrollment ----

    /**
     * Registers this phone with the relay if it has no credential yet, then polls
     * until a human claims it and tells the service to connect.
     *
     * Runs on a plain background thread rather than a coroutine: the app has no
     * coroutines dependency, and this is one long-lived poll loop, not structured
     * concurrency. It exits on claim, on a terminal error, or when replaced.
     */
    private fun startEnrollment() {
        // An explicitly configured URL wins — never override a QR/pasted setup.
        if (prefs.getString(AbacadAccessibilityService.KEY_SERVER_URL, "").orEmpty().isNotEmpty()) return
        if (enrollThread?.isAlive == true) return

        enrolling = true
        enrollError = null
        val t = Thread {
            var attempt = 0
            // Two passes at most: the second exists only for a stored credential
            // that turned out to be dead, so we discard it and enroll afresh
            // rather than making someone clear app data.
            while (attempt < 2 && !Thread.currentThread().isInterrupted) {
                attempt++
                val relay = Enrollment.normalizeRelay(
                    prefs.getString(AbacadAccessibilityService.KEY_RELAY_URL, "").orEmpty())
                var token = prefs.getString(AbacadAccessibilityService.KEY_DEVICE_TOKEN, "").orEmpty()

                if (token.isEmpty()) {
                    try {
                        val reg = Enrollment.register(relay, "android", defaultDeviceName(), BuildConfig.VERSION_NAME)
                        token = reg.deviceToken
                        prefs.edit()
                            .putString(AbacadAccessibilityService.KEY_RELAY_URL, relay)
                            .putString(AbacadAccessibilityService.KEY_DEVICE_ID, reg.deviceId)
                            .putString(AbacadAccessibilityService.KEY_DEVICE_TOKEN, reg.deviceToken)
                            .apply()
                        runOnUiThread {
                            deviceId = reg.deviceId
                            claimCode = reg.claimCode
                            relayUrl = relay
                        }
                    } catch (e: Enrollment.Failure.NotSupported) {
                        runOnUiThread {
                            enrolling = false
                            enrollError = "This relay doesn't support automatic setup. Use a connection URL below."
                        }
                        return@Thread
                    } catch (e: Exception) {
                        runOnUiThread {
                            enrolling = false
                            enrollError = "Couldn't reach ${Enrollment.normalizeRelay(relay)}. Check your network."
                        }
                        return@Thread
                    }
                }

                var transient = 0
                while (!Thread.currentThread().isInterrupted) {
                    try {
                        val st = Enrollment.heartbeat(relay, token)
                        transient = 0
                        if (st.claimed) {
                            runOnUiThread {
                                enrolling = false
                                claimCode = ""
                                toast(
                                    if (st.claimedBy.isEmpty()) "Added to your account"
                                    else "Added by ${st.claimedBy}")
                            }
                            // The service assembles the dial URL from relay_url +
                            // device_token, so it just needs a nudge.
                            sendBroadcast(
                                Intent(AbacadAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
                            return@Thread
                        }
                        runOnUiThread {
                            if (st.deviceId.isNotEmpty()) deviceId = st.deviceId
                            claimCode = st.claimCode
                        }
                        Thread.sleep(Enrollment.clampIntervalSec(st.heartbeatIn) * 1000L)
                    } catch (e: Enrollment.Failure.UnknownToken) {
                        // Registration reaped, or the device was deleted. Clear
                        // and go around once more to register from scratch.
                        prefs.edit()
                            .remove(AbacadAccessibilityService.KEY_DEVICE_ID)
                            .remove(AbacadAccessibilityService.KEY_DEVICE_TOKEN)
                            .apply()
                        runOnUiThread { deviceId = ""; claimCode = "" }
                        break
                    } catch (e: InterruptedException) {
                        return@Thread
                    } catch (e: Exception) {
                        // A flaky network shouldn't abandon a screen someone is
                        // reading a code off; retry, but don't spin forever.
                        transient++
                        if (transient >= 10) {
                            runOnUiThread {
                                enrolling = false
                                enrollError = "Lost contact with ${Enrollment.normalizeRelay(relay)}."
                            }
                            return@Thread
                        }
                        try { Thread.sleep(10_000L) } catch (_: InterruptedException) { return@Thread }
                    }
                }
            }
        }
        enrollThread = t
        t.isDaemon = true
        t.start()
    }

    /** A recognizable default name so the claim page shows more than "New device". */
    private fun defaultDeviceName(): String {
        val model = android.os.Build.MODEL.orEmpty().trim()
        return if (model.isEmpty()) "Android phone" else model
    }

    private fun disconnect() {
        sendBroadcast(Intent(AbacadAccessibilityService.ACTION_DISCONNECT).setPackage(packageName))
    }

    private fun togglePause() {
        AbacadStatus.setPaused(!AbacadStatus.paused)
    }

    // ---- status snapshot ------------------------------------------------------

    private data class Snapshot(
        val state: AbacadStatus.State,
        val detail: String,
        val paused: Boolean,
        val watched: Boolean,
        val recording: Boolean,
        val controlling: Boolean,
        val lastMethod: String?,
        val lines: List<AbacadStatus.Line>,
    )

    private fun snapshot() = Snapshot(
        state = AbacadStatus.state,
        detail = AbacadStatus.detail,
        paused = AbacadStatus.paused,
        watched = AbacadStatus.watched,
        recording = AbacadStatus.recording,
        controlling = AbacadStatus.controlling(),
        lastMethod = AbacadStatus.lastMethod,
        lines = AbacadStatus.recentLines(),
    )

    // ---- setup checklist (ported from the classic-View version) ---------------

    private fun buildChecks(): List<Check> {
        val checks = mutableListOf<Check>()

        checks += Check(
            title = "Accessibility service",
            evaluate = {
                if (isAccessibilityEnabled()) Level.OK to "Enabled"
                else Level.BAD to "Off — tap to enable (required)"
            },
            onTap = { safeStart(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)) },
        )

        checks += Check(
            title = "Battery optimization",
            evaluate = {
                if (isBatteryExempt()) Level.OK to "Exempt — survives Doze off-charger"
                else Level.WARN to "Not exempt — tap to allow (drops on sleep)"
            },
            onTap = {
                if (isBatteryExempt()) toast("Already exempt from battery optimization")
                else safeStart(
                    Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS, Uri.parse("package:$packageName")),
                )
            },
        )

        checks += Check(
            title = "Display over other apps",
            evaluate = {
                if (Settings.canDrawOverlays(this)) Level.OK to "Allowed — auto-wake reliable"
                else Level.WARN to "Not allowed — auto-wake may fail on strict ROMs"
            },
            onTap = { safeStart(Intent(Settings.ACTION_MANAGE_OVERLAY_PERMISSION, Uri.parse("package:$packageName"))) },
        )

        checks += Check(
            title = "Files & media access",
            evaluate = {
                if (Environment.isExternalStorageManager()) Level.OK to "Granted — can save to any folder (Pictures, Download…)"
                else Level.WARN to "Off — tap to allow saving files to shared storage"
            },
            onTap = {
                if (Environment.isExternalStorageManager()) {
                    toast("Already granted — files can be saved anywhere in shared storage")
                } else {
                    try {
                        startActivity(
                            Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION, Uri.parse("package:$packageName")),
                        )
                    } catch (_: ActivityNotFoundException) {
                        safeStart(Intent(Settings.ACTION_MANAGE_ALL_FILES_ACCESS_PERMISSION))
                    }
                }
            },
        )

        if (Build.VERSION.SDK_INT >= 33) {
            checks += Check(
                title = "Notifications",
                evaluate = {
                    if (notificationsGranted()) Level.OK to "Allowed — connection status shows"
                    else Level.WARN to "Off — the ongoing status won't show"
                },
                onTap = {
                    if (notificationsGranted()) {
                        safeStart(
                            Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS).putExtra(Settings.EXTRA_APP_PACKAGE, packageName),
                        )
                    } else {
                        notifPermLauncher.launch(android.Manifest.permission.POST_NOTIFICATIONS)
                    }
                },
            )
        }

        checks += Check(
            title = "Screen lock",
            evaluate = {
                if (isDeviceSecure()) Level.WARN to "Secure lock — can't auto-unlock when asleep"
                else Level.OK to "None/Swipe — auto-unlock OK"
            },
            onTap = { safeStart(Intent(Settings.ACTION_SECURITY_SETTINGS)) },
        )

        if (Build.MANUFACTURER.equals("samsung", ignoreCase = true)) {
            checks += Check(
                title = "Never sleeping apps (Samsung)",
                evaluate = { Level.WARN to "Add abacad here so One UI won't freeze it" },
                onTap = { safeStart(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.parse("package:$packageName"))) },
            )
        }

        return checks
    }

    private fun isAccessibilityEnabled(): Boolean {
        val me = ComponentName(this, AbacadAccessibilityService::class.java)
        val enabled = Settings.Secure.getString(contentResolver, Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES) ?: return false
        return enabled.split(':').any { ComponentName.unflattenFromString(it) == me }
    }

    private fun isBatteryExempt(): Boolean =
        (getSystemService(Context.POWER_SERVICE) as PowerManager).isIgnoringBatteryOptimizations(packageName)

    private fun isDeviceSecure(): Boolean =
        (getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager).isDeviceSecure

    private fun notificationsGranted(): Boolean =
        Build.VERSION.SDK_INT < 33 ||
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    private fun safeStart(intent: Intent) {
        try {
            startActivity(intent)
        } catch (_: ActivityNotFoundException) {
            toast("This setting isn't available on this device — open Settings manually.")
        }
    }

    private fun toast(msg: String) = Toast.makeText(this, msg, Toast.LENGTH_SHORT).show()

    // ---- composables ----------------------------------------------------------

    private fun levelColor(level: Level, c: AbacadColors): Color = when (level) {
        Level.OK -> c.success
        Level.WARN -> c.warning
        Level.BAD -> c.danger
        Level.INFO -> c.inkSubtle
    }

    @Composable
    private fun StateBanner(
        s: Snapshot,
        c: AbacadColors,
        ready: Boolean,
        onPause: () -> Unit,
        onDisconnect: () -> Unit,
    ) {
        val (dot, title, subtitle) = when {
            s.paused -> Triple(c.warning, "Paused", "the agent can't touch this device until you resume")
            s.controlling -> Triple(c.success, "Controlling now", "agent · ${s.lastMethod ?: "running"}")
            s.state == AbacadStatus.State.CONNECTED -> Triple(c.success, "Connected", "idle — no agent active")
            s.state == AbacadStatus.State.CONNECTING -> Triple(c.warning, "Connecting", s.detail)
            s.state == AbacadStatus.State.RECONNECTING -> Triple(c.warning, "Reconnecting", s.detail)
            else -> Triple(c.inkSubtle, "Disconnected", s.detail)
        }
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            shape = RoundedCornerShape(AbacadDim.radiusMd),
        ) {
            // Status and actions are stacked, not side-by-side: giving the title
            // its own full-width row keeps "Controlling now" / "agent · screenshot"
            // on single lines instead of being squeezed to a two-word column.
            Column(Modifier.fillMaxWidth().padding(AbacadDim.spaceMd)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(10.dp).clip(CircleShape).background(dot))
                    Spacer(Modifier.width(AbacadDim.spaceMd))
                    Column(Modifier.weight(1f)) {
                        Text(title, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                        Text(subtitle, color = c.inkMuted, style = MaterialTheme.typography.bodySmall)
                    }
                }
                if (ready) {
                    Spacer(Modifier.height(AbacadDim.spaceMd))
                    Row(
                        Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(AbacadDim.spaceSm),
                    ) {
                        OutlinedButton(onClick = onPause, modifier = Modifier.weight(1f)) {
                            Text(if (s.paused) "Resume control" else "Pause control")
                        }
                        Button(
                            onClick = onDisconnect,
                            modifier = Modifier.weight(1f),
                            colors = ButtonDefaults.buttonColors(containerColor = c.danger, contentColor = Color.White),
                        ) { Text("Disconnect") }
                    }
                }
            }
        }
    }

    @Composable
    private fun AwarenessFlags(s: Snapshot, c: AbacadColors) {
        if (!s.watched && !s.recording) return
        Spacer(Modifier.height(AbacadDim.spaceSm))
        Row(horizontalArrangement = Arrangement.spacedBy(AbacadDim.spaceSm)) {
            if (s.watched) Flag("👁  Screen being watched", c.warning, c.warningSoft)
            if (s.recording) Flag("●  Recording", c.danger, c.dangerSoft)
        }
    }

    @Composable
    private fun Flag(text: String, fg: Color, bg: Color) {
        Box(
            Modifier.clip(RoundedCornerShape(AbacadDim.radiusPill)).background(bg)
                .padding(horizontal = AbacadDim.spaceMd, vertical = AbacadDim.spaceXs),
        ) { Text(text, color = fg, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold) }
    }

    @Composable
    private fun RecentActions(s: Snapshot, c: AbacadColors) {
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            shape = RoundedCornerShape(AbacadDim.radiusMd),
        ) {
            Column(Modifier.fillMaxWidth().padding(AbacadDim.spaceMd)) {
                if (s.lines.isEmpty()) {
                    Text("No activity yet.", color = c.inkMuted, style = MaterialTheme.typography.bodySmall)
                } else {
                    s.lines.asReversed().take(12).forEach { line ->
                        Row(Modifier.fillMaxWidth().padding(vertical = 3.dp)) {
                            Text(
                                DateFormat.format("HH:mm:ss", Date(line.ts)).toString(),
                                color = c.inkSubtle,
                                fontFamily = FontFamily.Monospace,
                                style = MaterialTheme.typography.labelSmall,
                            )
                            Spacer(Modifier.width(AbacadDim.spaceSm))
                            Text(line.text, color = MaterialTheme.colorScheme.onSurface, style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            }
        }
    }

    /**
     * The two lines a human reads off this phone and types at <relay>/claim.
     * Replaces "paste a connection URL" as the primary setup step.
     */
    @Composable
    private fun ClaimSection(
        deviceId: String,
        claimCode: String,
        relay: String,
        enrolling: Boolean,
        error: String?,
        palette: AbacadColors,
        onRetry: () -> Unit,
    ) {
        SectionLabel("Add this phone")
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            shape = RoundedCornerShape(AbacadDim.radiusMd),
        ) {
            Column(Modifier.fillMaxWidth().padding(AbacadDim.spaceLg)) {
                when {
                    error != null -> {
                        Text(error, color = palette.danger, style = MaterialTheme.typography.bodySmall)
                        Spacer(Modifier.height(AbacadDim.spaceSm))
                        OutlinedButton(onClick = onRetry) { Text("Try again") }
                    }
                    claimCode.isEmpty() && enrolling -> {
                        Text(
                            "Contacting ${Enrollment.normalizeRelay(relay)}…",
                            color = palette.inkMuted,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                    claimCode.isNotEmpty() -> {
                        Text("Device ID", color = palette.inkMuted, style = MaterialTheme.typography.labelSmall)
                        Text(
                            deviceId,
                            color = MaterialTheme.colorScheme.onSurface,
                            fontFamily = FontFamily.Monospace,
                            style = MaterialTheme.typography.bodyLarge,
                        )
                        Spacer(Modifier.height(AbacadDim.spaceSm))
                        Text("Claim code", color = palette.inkMuted, style = MaterialTheme.typography.labelSmall)
                        Text(
                            claimCode,
                            color = MaterialTheme.colorScheme.onSurface,
                            fontFamily = FontFamily.Monospace,
                            fontWeight = FontWeight.Bold,
                            style = MaterialTheme.typography.headlineSmall,
                        )
                        Spacer(Modifier.height(AbacadDim.spaceSm))
                        Text(
                            "Open ${Enrollment.normalizeRelay(relay)}/claim and enter both.",
                            color = palette.inkMuted,
                            style = MaterialTheme.typography.bodySmall,
                        )
                        Spacer(Modifier.height(AbacadDim.spaceXs))
                        Text(
                            "The code changes every few minutes. Anyone who can read these two lines can add this phone to their account.",
                            color = palette.inkSubtle,
                            style = MaterialTheme.typography.labelSmall,
                        )
                    }
                    else -> {
                        Text(
                            "Not connected to a relay yet.",
                            color = palette.inkMuted,
                            style = MaterialTheme.typography.bodySmall,
                        )
                        Spacer(Modifier.height(AbacadDim.spaceSm))
                        OutlinedButton(onClick = onRetry) { Text("Set up") }
                    }
                }
            }
        }
    }

    @Composable
    private fun ConnectSection(url: String, onUrl: (String) -> Unit, onScan: () -> Unit, onConnect: () -> Unit) {
        SectionLabel("Other ways to connect")
        OutlinedTextField(
            value = url,
            onValueChange = onUrl,
            label = { Text("Server URL") },
            placeholder = { Text("ws://<server-ip>:8848/device") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
            modifier = Modifier.fillMaxWidth(),
        )
        Spacer(Modifier.height(AbacadDim.spaceSm))
        Row(horizontalArrangement = Arrangement.spacedBy(AbacadDim.spaceSm)) {
            OutlinedButton(onClick = onScan, modifier = Modifier.weight(1f)) { Text("Scan QR") }
            Button(onClick = onConnect, modifier = Modifier.weight(1f)) { Text("Save & Connect") }
        }
    }

    @Composable
    private fun SetupDrawer(checks: List<RenderedCheck>, c: AbacadColors, startExpanded: Boolean) {
        var expanded by remember { mutableStateOf(startExpanded) }
        val attention = checks.count { it.level != Level.OK }
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            shape = RoundedCornerShape(AbacadDim.radiusMd),
        ) {
            Column(Modifier.fillMaxWidth()) {
                Row(
                    Modifier.fillMaxWidth().clickable { expanded = !expanded }.padding(AbacadDim.spaceMd),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("Setup", color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.width(AbacadDim.spaceSm))
                    Text(
                        if (attention == 0) "✓ all set" else "$attention needs attention",
                        color = if (attention == 0) c.success else c.warning,
                        style = MaterialTheme.typography.bodySmall,
                    )
                    Spacer(Modifier.weight(1f))
                    Text(if (expanded) "▾" else "▸", color = c.inkSubtle)
                }
                if (expanded) {
                    Column(Modifier.padding(horizontal = AbacadDim.spaceMd).padding(bottom = AbacadDim.spaceSm)) {
                        checks.forEach { CheckRow(it, c) }
                    }
                }
            }
        }
    }

    @Composable
    private fun Checklist(checks: List<RenderedCheck>, c: AbacadColors) {
        Card(
            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
            shape = RoundedCornerShape(AbacadDim.radiusMd),
        ) {
            Column(Modifier.fillMaxWidth().padding(AbacadDim.spaceMd)) {
                checks.forEach { CheckRow(it, c) }
            }
        }
    }

    @Composable
    private fun CheckRow(row: RenderedCheck, c: AbacadColors) {
        val base = Modifier.fillMaxWidth()
        val clickable = if (row.onTap != null) base.clickable { row.onTap.invoke() } else base
        Row(
            clickable.padding(vertical = AbacadDim.spaceSm),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(Modifier.size(9.dp).clip(CircleShape).background(levelColor(row.level, c)))
            Spacer(Modifier.width(AbacadDim.spaceMd))
            Column(Modifier.weight(1f)) {
                Text(row.title, color = MaterialTheme.colorScheme.onSurface, style = MaterialTheme.typography.bodyMedium)
                Text(row.detail, color = c.inkMuted, style = MaterialTheme.typography.bodySmall)
            }
            if (row.onTap != null) Text("›", color = c.inkSubtle)
        }
    }

    @Composable
    private fun SectionLabel(text: String) {
        Text(
            text.uppercase(),
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            fontFamily = FontFamily.Monospace,
            style = MaterialTheme.typography.labelSmall,
            modifier = Modifier.padding(bottom = AbacadDim.spaceSm),
        )
    }

    @Composable
    private fun SetupHelp(c: AbacadColors) {
        Text(
            "Scan the connection QR on the abacad dashboard, or type ws://<server-ip>:8848/device " +
                "(server and phone on the same Wi-Fi), then tap Save & Connect. Work the Setup list until " +
                "every dot is green — Accessibility is required; the rest keep the connection alive while the " +
                "screen sleeps. Once connected, an agent can read the screen, type, tap/swipe, and screenshot " +
                "this device; you can Pause or Disconnect here at any time.",
            color = c.inkMuted,
            style = MaterialTheme.typography.bodySmall,
        )
    }
}
