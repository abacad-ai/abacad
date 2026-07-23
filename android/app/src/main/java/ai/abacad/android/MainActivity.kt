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

    private val statusListener: () -> Unit = { runOnUiThread { statusTick++ } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        url = prefs.getString(AbacadAccessibilityService.KEY_SERVER_URL, "").orEmpty()

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
            s.paused -> Triple(c.warning, "Paused", "commands are being rejected on this device")
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
            Row(
                modifier = Modifier.fillMaxWidth().padding(AbacadDim.spaceMd),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(Modifier.size(10.dp).clip(CircleShape).background(dot))
                Spacer(Modifier.width(AbacadDim.spaceMd))
                Column(Modifier.weight(1f)) {
                    Text(title, color = MaterialTheme.colorScheme.onSurface, fontWeight = FontWeight.Bold)
                    Text(subtitle, color = c.inkMuted, style = MaterialTheme.typography.bodySmall)
                }
                if (ready) {
                    OutlinedButton(onClick = onPause) { Text(if (s.paused) "Resume" else "Pause") }
                    Spacer(Modifier.width(AbacadDim.spaceSm))
                    Button(
                        onClick = onDisconnect,
                        colors = ButtonDefaults.buttonColors(containerColor = c.danger, contentColor = Color.White),
                    ) { Text("Disconnect") }
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

    @Composable
    private fun ConnectSection(url: String, onUrl: (String) -> Unit, onScan: () -> Unit, onConnect: () -> Unit) {
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
