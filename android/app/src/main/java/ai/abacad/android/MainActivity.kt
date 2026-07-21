package ai.abacad.android

import android.app.Activity
import android.app.KeyguardManager
import android.content.ActivityNotFoundException
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Typeface
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.PowerManager
import android.provider.Settings
import android.text.format.DateFormat
import android.view.Gravity
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import java.util.Date

/**
 * Minimal setup screen: enter the abacad server URL, save it, enable the
 * accessibility service, and grant the handful of permissions that let the
 * device survive screen-off idle. All control happens over the network
 * afterward; this screen only configures the connection.
 *
 * Two live panels drive it, both refreshed whenever the screen is shown:
 *  - a connection-status panel fed by [AbacadStatus] (connected / reconnecting /
 *    stuck, plus recent command activity) — so the user doesn't reach for adb;
 *  - a setup checklist that reads the *current* state of each capability
 *    (accessibility on? battery-exempt? overlay? …) and shows a colored dot per
 *    row, each row tappable to jump straight to the system screen that fixes it.
 *    The checklist re-evaluates in [onResume], so returning from a Settings page
 *    reflects the change immediately.
 */
class MainActivity : Activity() {

    private companion object {
        const val REQ_SCAN = 1
        const val REQ_NOTIF = 2
    }

    /** Severity of a checklist row, mapped to the dot color. */
    private enum class Level { OK, WARN, BAD, INFO }

    /**
     * One setup requirement: how to read its current state ([evaluate] → level +
     * a short human detail) and, optionally, what tapping the row does ([onTap]).
     */
    private class Check(
        val title: String,
        val evaluate: () -> Pair<Level, String>,
        val onTap: (() -> Unit)?,
    )

    /** A rendered checklist row: its check, its dot view, its detail view. */
    private class Row(val check: Check, val dot: TextView, val detail: TextView)

    private lateinit var urlField: EditText
    private lateinit var statusView: TextView
    private lateinit var activityView: TextView
    private val rows = mutableListOf<Row>()

    // Resolved in onCreate; the activity is recreated on a uiMode (dark/light) change.
    private lateinit var theme: Theme.Palette

    // Re-render the panel whenever AbacadStatus changes (called off the UI thread).
    private val statusListener: () -> Unit = { runOnUiThread { renderStatus() } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val prefs = getSharedPreferences(AbacadAccessibilityService.PREFS, Context.MODE_PRIVATE)
        theme = Theme.of(resources)
        val dp = resources.displayMetrics.density
        val pad = (Theme.SPACE_XL * dp).toInt()

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }

        // --- live connection status panel (top of screen) ---
        val statusHeader = sectionHeader("Connection")
        statusView = TextView(this).apply {
            textSize = Theme.TEXT_MD
            setPadding(0, (Theme.SPACE_XS * dp).toInt(), 0, 0)
        }
        activityView = TextView(this).apply {
            textSize = Theme.TEXT_XS
            typeface = Typeface.MONOSPACE
            setTextColor(theme.INK_SUBTLE)
            setPadding(0, (Theme.SPACE_SM * dp).toInt(), 0, pad)
        }

        urlField = EditText(this).apply {
            hint = "ws://<server-ip>:8848/device"
            setText(prefs.getString(AbacadAccessibilityService.KEY_SERVER_URL, ""))
        }

        val scanBtn = Button(this).apply {
            text = "Scan QR"
            setOnClickListener {
                startActivityForResult(Intent(this@MainActivity, ScanActivity::class.java), REQ_SCAN)
            }
        }

        val connectBtn = Button(this).apply {
            text = "Save & Connect"
            setOnClickListener {
                val url = urlField.text.toString().trim()
                prefs.edit().putString(AbacadAccessibilityService.KEY_SERVER_URL, url).apply()
                sendBroadcast(Intent(AbacadAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
                // Don't echo the URL — it carries the device token.
                Toast.makeText(this@MainActivity, "Saved. Connecting…", Toast.LENGTH_SHORT).show()
            }
        }

        // --- setup checklist: one tappable row per capability, colored by state ---
        val setupHeader = sectionHeader("Setup").apply {
            setPadding(0, pad, 0, 0)
        }
        val checklist = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        for (check in buildChecks()) checklist.addView(buildRow(check, dp))

        val info = TextView(this).apply {
            textSize = Theme.TEXT_SM
            setTextColor(theme.INK_MUTED)
            text = """
                abacad — device agent

                1. Tap Scan QR and point at the connection QR on the abacad
                   dashboard — or type ws://<server-ip>:8848/device by hand
                   (server machine + this phone on the same Wi-Fi).
                2. Tap Save & Connect (scanning connects for you).
                3. Work down the Setup list above until every dot is green.
                   Accessibility is required; the rest keep the connection alive
                   while the screen sleeps.

                A green dot means the requirement is met right now; amber/red means
                tap the row to fix it. "Never sleeping apps" can't be read back, so
                it stays amber — tap it and add abacad there once.

                Once connected, an agent (via the server) can read the screen, type,
                inject taps/swipes, and screenshot this device. It stays connected
                while the screen sleeps; the agent wakes the screen automatically
                when it needs it (a one-time ~few-second cost), then runs at normal
                latency. See docs/power-lockscreen.md for the full support matrix.

                Logs:  adb logcat -s ABACAD
            """.trimIndent()
        }

        root.setBackgroundColor(theme.CANVAS)
        root.addView(statusHeader)
        root.addView(statusView)
        root.addView(activityView)
        root.addView(urlField)
        root.addView(scanBtn)
        root.addView(connectBtn)
        root.addView(setupHeader)
        root.addView(checklist)
        root.addView(info)
        setContentView(ScrollView(this).apply { addView(root) })

        // Android 13+: the foreground-service notification needs POST_NOTIFICATIONS to be visible.
        // The service still runs without it, but the ongoing notification is the user's signal that
        // the device is connected, so ask once up front.
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), REQ_NOTIF)
        }
    }

    // ---- setup checklist ------------------------------------------------------

    /**
     * The requirements shown in the Setup list, in priority order: accessibility
     * (required) first, then the keep-alive grants, then advisory rows. Each
     * [Check.evaluate] reads live system state so the row repaints on [onResume].
     */
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
                if (isBatteryExempt()) {
                    toast("Already exempt from battery optimization")
                } else {
                    safeStart(
                        Intent(
                            Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                            Uri.parse("package:$packageName"),
                        ),
                    )
                }
            },
        )

        checks += Check(
            title = "Display over other apps",
            evaluate = {
                if (Settings.canDrawOverlays(this)) Level.OK to "Allowed — auto-wake reliable"
                else Level.WARN to "Not allowed — auto-wake may fail on strict ROMs"
            },
            onTap = {
                safeStart(
                    Intent(
                        Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                        Uri.parse("package:$packageName"),
                    ),
                )
            },
        )

        // Below 33 notifications are on by default; only worth a row on 13+.
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
                            Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
                                .putExtra(Settings.EXTRA_APP_PACKAGE, packageName),
                        )
                    } else {
                        requestPermissions(
                            arrayOf(android.Manifest.permission.POST_NOTIFICATIONS),
                            REQ_NOTIF,
                        )
                    }
                },
            )
        }

        checks += Check(
            title = "Screen lock",
            evaluate = {
                if (isDeviceSecure()) {
                    Level.WARN to "Secure lock — can't auto-unlock when asleep"
                } else {
                    Level.OK to "None/Swipe — auto-unlock OK"
                }
            },
            onTap = { safeStart(Intent(Settings.ACTION_SECURITY_SETTINGS)) },
        )

        // One UI freezes background apps regardless of the grants above unless the
        // app is on its "Never sleeping apps" allowlist — which has no public
        // getter, so this row is always advisory (amber) and just deep-links out.
        if (Build.MANUFACTURER.equals("samsung", ignoreCase = true)) {
            checks += Check(
                title = "Never sleeping apps (Samsung)",
                evaluate = { Level.WARN to "Add abacad here so One UI won't freeze it" },
                onTap = {
                    safeStart(
                        Intent(
                            Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                            Uri.parse("package:$packageName"),
                        ),
                    )
                },
            )
        }

        return checks
    }

    /** Build a tappable checklist row (dot + title/detail); registers it for refresh. */
    private fun buildRow(check: Check, dp: Float): LinearLayout {
        val dot = TextView(this).apply {
            text = "●"
            textSize = Theme.TEXT_MD
            setPadding(0, 0, (Theme.SPACE_MD * dp).toInt(), 0)
        }
        val title = TextView(this).apply {
            text = check.title
            textSize = Theme.TEXT_MD
            setTextColor(theme.INK)
        }
        val detail = TextView(this).apply {
            textSize = Theme.TEXT_XS
            setTextColor(theme.INK_MUTED)
        }
        val text = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f)
            addView(title)
            addView(detail)
        }
        rows += Row(check, dot, detail)
        return LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            val v = (Theme.SPACE_MD * dp).toInt()
            setPadding(0, v, 0, v)
            addView(dot)
            addView(text)
            check.onTap?.let { tap -> isClickable = true; setOnClickListener { tap() } }
        }
    }

    /** Re-read every checklist row's live state and repaint its dot + detail. */
    private fun refreshChecks() {
        for (row in rows) {
            val (level, detail) = row.check.evaluate()
            row.dot.setTextColor(
                when (level) {
                    Level.OK -> theme.SUCCESS
                    Level.WARN -> theme.WARNING
                    Level.BAD -> theme.DANGER
                    Level.INFO -> theme.INK_SUBTLE
                },
            )
            row.detail.text = detail
        }
    }

    private fun isAccessibilityEnabled(): Boolean {
        val me = ComponentName(this, AbacadAccessibilityService::class.java)
        val enabled = Settings.Secure.getString(
            contentResolver,
            Settings.Secure.ENABLED_ACCESSIBILITY_SERVICES,
        ) ?: return false
        return enabled.split(':').any { ComponentName.unflattenFromString(it) == me }
    }

    private fun isBatteryExempt(): Boolean =
        (getSystemService(Context.POWER_SERVICE) as PowerManager).isIgnoringBatteryOptimizations(packageName)

    private fun isDeviceSecure(): Boolean =
        (getSystemService(Context.KEYGUARD_SERVICE) as KeyguardManager).isDeviceSecure

    private fun notificationsGranted(): Boolean =
        Build.VERSION.SDK_INT < 33 ||
            checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    /** Start a settings intent, tolerating OEMs that don't expose it. */
    private fun safeStart(intent: Intent) {
        try {
            startActivity(intent)
        } catch (_: ActivityNotFoundException) {
            toast("This setting isn't available on this device — open Settings manually.")
        }
    }

    private fun toast(msg: String) = Toast.makeText(this, msg, Toast.LENGTH_SHORT).show()

    private fun sectionHeader(text: String) = TextView(this).apply {
        this.text = text
        textSize = Theme.TEXT_LG
        setTextColor(theme.INK)
        setTypeface(typeface, Typeface.BOLD)
    }

    override fun onResume() {
        super.onResume()
        AbacadStatus.addListener(statusListener)
        renderStatus()
        refreshChecks()
    }

    override fun onPause() {
        super.onPause()
        AbacadStatus.removeListener(statusListener)
    }

    /** Paint the current [AbacadStatus] state (colored headline) and recent activity. */
    private fun renderStatus() {
        val s = AbacadStatus.state
        statusView.text = "● ${s.name.lowercase()} — ${AbacadStatus.detail}"
        statusView.setTextColor(
            when (s) {
                AbacadStatus.State.CONNECTED -> theme.SUCCESS
                AbacadStatus.State.CONNECTING, AbacadStatus.State.RECONNECTING -> theme.WARNING
                AbacadStatus.State.DISCONNECTED -> theme.DANGER
            },
        )
        val lines = AbacadStatus.recentLines()
        activityView.text = if (lines.isEmpty()) {
            "No activity yet."
        } else {
            // Newest first, capped so the panel stays readable.
            lines.asReversed().take(12).joinToString("\n") { line ->
                "${DateFormat.format("HH:mm:ss", Date(line.ts))}  ${line.text}"
            }
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQ_SCAN || resultCode != RESULT_OK) return
        val url = data?.getStringExtra(ScanActivity.RESULT_TEXT)?.trim().orEmpty()
        if (url.isEmpty()) return
        urlField.setText(url)
        // Scanning is an explicit "connect me" gesture — save + reconnect straight away.
        getSharedPreferences(AbacadAccessibilityService.PREFS, Context.MODE_PRIVATE)
            .edit().putString(AbacadAccessibilityService.KEY_SERVER_URL, url).apply()
        sendBroadcast(Intent(AbacadAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
        // Don't echo the URL — it carries the device token.
        Toast.makeText(this, "Scanned. Connecting…", Toast.LENGTH_SHORT).show()
    }
}
