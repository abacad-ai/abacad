package ai.abacad.android

import android.app.Activity
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
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast
import java.util.Date

/**
 * Minimal setup screen: enter the abacad server URL, save it, and enable the
 * accessibility service. All control happens over the network afterward; this
 * screen only configures the connection.
 *
 * It also shows a live connection-status panel fed by [AbacadStatus], so the
 * user can see whether the device is connected, reconnecting, or stuck — and the
 * recent command/error activity — without reaching for `adb logcat`.
 */
class MainActivity : Activity() {

    private companion object {
        const val REQ_SCAN = 1
        const val REQ_NOTIF = 2
    }

    private lateinit var urlField: EditText
    private lateinit var statusView: TextView
    private lateinit var activityView: TextView

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
        val statusHeader = TextView(this).apply {
            text = "Connection"
            textSize = Theme.TEXT_LG
            setTextColor(theme.INK)
            setTypeface(typeface, Typeface.BOLD)
        }
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

        val a11yBtn = Button(this).apply {
            text = "Open Accessibility Settings"
            setOnClickListener { startActivity(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS)) }
        }

        // "Display over other apps" makes the auto-wake background-launch reliable on strict ROMs.
        val overlayBtn = Button(this).apply {
            text = "Allow Display Over Other Apps"
            setOnClickListener {
                startActivity(
                    Intent(
                        Settings.ACTION_MANAGE_OVERLAY_PERMISSION,
                        Uri.parse("package:$packageName"),
                    ),
                )
            }
        }

        // Battery-optimization exemption keeps the held socket alive through Doze off-charger; the
        // foreground service does the rest. Deep-links straight to the system grant.
        val batteryBtn = Button(this).apply {
            text = "Ignore Battery Optimization"
            setOnClickListener {
                val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
                if (pm.isIgnoringBatteryOptimizations(packageName)) {
                    Toast.makeText(this@MainActivity, "Already exempt from battery optimization", Toast.LENGTH_SHORT).show()
                } else {
                    startActivity(
                        Intent(
                            Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS,
                            Uri.parse("package:$packageName"),
                        ),
                    )
                }
            }
        }

        val info = TextView(this).apply {
            textSize = Theme.TEXT_SM
            setTextColor(theme.INK_MUTED)
            text = """
                abacad — device agent

                1. Tap Scan QR and point at the connection QR on the
                   abacad dashboard — or type the URL by hand:
                   ws://<server-ip>:8848/device
                   (server machine + this phone on the same Wi-Fi)
                2. Tap Save & Connect (scanning connects for you).
                3. Enable "abacad" under Accessibility (button below);
                   accept the system warning.

                Once connected, an agent (via the server) can read the screen,
                type, inject taps/swipes, and screenshot this device.

                For hands-off, screen-off idle (optional):
                  • Set the screen lock to None or Swipe (NOT a PIN/pattern —
                    a secure lock cannot be auto-unlocked).
                  • Tap "Allow Display Over Other Apps" so auto-wake can turn the
                    screen back on reliably on strict OEM ROMs.
                  • Tap "Ignore Battery Optimization" so the connection survives
                    Doze while the screen is off.
                  • Samsung only: Settings → Battery → Background usage limits →
                    add abacad to "Never sleeping apps" (One UI will otherwise
                    sleep the app and drop the connection).
                The device stays connected while its screen sleeps; the agent wakes
                the screen automatically when it needs it (a one-time ~few-second cost),
                then runs at normal latency.
                See docs/power-lockscreen.md for the full support matrix.

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
        root.addView(a11yBtn)
        root.addView(overlayBtn)
        root.addView(batteryBtn)
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

    override fun onResume() {
        super.onResume()
        AbacadStatus.addListener(statusListener)
        renderStatus()
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
