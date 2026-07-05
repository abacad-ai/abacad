package dev.abacad.probe

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.graphics.Typeface
import android.net.Uri
import android.os.Bundle
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
 * Minimal setup screen: enter the Abacad server URL, save it, and enable the
 * accessibility service. All control happens over the network afterward; this
 * screen only configures the connection.
 *
 * It also shows a live connection-status panel fed by [ProbeStatus], so the
 * user can see whether the device is connected, reconnecting, or stuck — and the
 * recent command/error activity — without reaching for `adb logcat`.
 */
class MainActivity : Activity() {

    private companion object {
        const val REQ_SCAN = 1
    }

    private lateinit var urlField: EditText
    private lateinit var statusView: TextView
    private lateinit var activityView: TextView

    // Re-render the panel whenever ProbeStatus changes (called off the UI thread).
    private val statusListener: () -> Unit = { runOnUiThread { renderStatus() } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val prefs = getSharedPreferences(ProbeAccessibilityService.PREFS, Context.MODE_PRIVATE)
        val pad = (24 * resources.displayMetrics.density).toInt()

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }

        // --- live connection status panel (top of screen) ---
        val statusHeader = TextView(this).apply {
            text = "Connection"
            setTypeface(typeface, Typeface.BOLD)
        }
        statusView = TextView(this).apply {
            textSize = 15f
            setPadding(0, (4 * resources.displayMetrics.density).toInt(), 0, 0)
        }
        activityView = TextView(this).apply {
            textSize = 12f
            typeface = Typeface.MONOSPACE
            setTextColor(0xFF64748B.toInt())
            setPadding(0, (6 * resources.displayMetrics.density).toInt(), 0, pad)
        }

        urlField = EditText(this).apply {
            hint = "ws://<server-ip>:8848/device"
            setText(prefs.getString(ProbeAccessibilityService.KEY_SERVER_URL, ""))
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
                prefs.edit().putString(ProbeAccessibilityService.KEY_SERVER_URL, url).apply()
                sendBroadcast(Intent(ProbeAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
                Toast.makeText(this@MainActivity, "Saved. Connecting to $url", Toast.LENGTH_SHORT).show()
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

        val info = TextView(this).apply {
            textSize = 13f
            text = """
                Abacad — device agent

                1. Tap Scan QR and point at the connection QR on the
                   Abacad dashboard — or type the URL by hand:
                   ws://<server-ip>:8848/device
                   (server machine + this phone on the same Wi-Fi)
                2. Tap Save & Connect (scanning connects for you).
                3. Enable "Abacad Probe" under Accessibility (button below);
                   accept the system warning.

                Once connected, an agent (via the server) can read the screen,
                type, inject taps/swipes, and screenshot this device.

                For hands-off, screen-off idle (optional):
                  • Set the screen lock to None or Swipe (NOT a PIN/pattern —
                    a secure lock cannot be auto-unlocked).
                  • Tap "Allow Display Over Other Apps" so auto-wake can turn the
                    screen back on reliably on strict OEM ROMs.
                The device sleeps on its own display timeout; the agent wakes it
                automatically when it needs the screen.
                See docs/power-lockscreen.md for the full support matrix.

                Logs:  adb logcat -s ABACAD
            """.trimIndent()
        }

        root.addView(statusHeader)
        root.addView(statusView)
        root.addView(activityView)
        root.addView(urlField)
        root.addView(scanBtn)
        root.addView(connectBtn)
        root.addView(a11yBtn)
        root.addView(overlayBtn)
        root.addView(info)
        setContentView(ScrollView(this).apply { addView(root) })
    }

    override fun onResume() {
        super.onResume()
        ProbeStatus.addListener(statusListener)
        renderStatus()
    }

    override fun onPause() {
        super.onPause()
        ProbeStatus.removeListener(statusListener)
    }

    /** Paint the current [ProbeStatus] state (colored headline) and recent activity. */
    private fun renderStatus() {
        val s = ProbeStatus.state
        statusView.text = "● ${s.name.lowercase()} — ${ProbeStatus.detail}"
        statusView.setTextColor(
            when (s) {
                ProbeStatus.State.CONNECTED -> 0xFF16A34A.toInt() // green
                ProbeStatus.State.CONNECTING, ProbeStatus.State.RECONNECTING -> 0xFFCA8A04.toInt() // amber
                ProbeStatus.State.DISCONNECTED -> 0xFFDC2626.toInt() // red
            },
        )
        val lines = ProbeStatus.recentLines()
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
        getSharedPreferences(ProbeAccessibilityService.PREFS, Context.MODE_PRIVATE)
            .edit().putString(ProbeAccessibilityService.KEY_SERVER_URL, url).apply()
        sendBroadcast(Intent(ProbeAccessibilityService.ACTION_RECONNECT).setPackage(packageName))
        Toast.makeText(this, "Scanned. Connecting to $url", Toast.LENGTH_SHORT).show()
    }
}
