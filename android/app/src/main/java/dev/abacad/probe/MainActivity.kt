package dev.abacad.probe

import android.app.Activity
import android.app.admin.DevicePolicyManager
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.provider.Settings
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import android.widget.Toast

/**
 * Minimal setup screen: enter the Abacad server URL, save it, and enable the
 * accessibility service. All control happens over the network afterward; this
 * screen only configures the connection.
 */
class MainActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val prefs = getSharedPreferences(ProbeAccessibilityService.PREFS, Context.MODE_PRIVATE)
        val pad = (24 * resources.displayMetrics.density).toInt()

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }

        val urlField = EditText(this).apply {
            hint = "ws://<server-ip>:8848/device"
            setText(prefs.getString(ProbeAccessibilityService.KEY_SERVER_URL, ""))
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

        // Optional, for screen-off idle: device admin lets `sleep` turn the screen off;
        // "Display over other apps" makes the `wake` background-launch reliable on strict ROMs.
        val adminBtn = Button(this).apply {
            text = "Enable Screen Off (device admin)"
            setOnClickListener {
                val admin = ComponentName(this@MainActivity, AbacadDeviceAdmin::class.java)
                val intent = Intent(DevicePolicyManager.ACTION_ADD_DEVICE_ADMIN)
                    .putExtra(DevicePolicyManager.EXTRA_DEVICE_ADMIN, admin)
                    .putExtra(
                        DevicePolicyManager.EXTRA_ADD_EXPLANATION,
                        "Lets Abacad turn the screen off between tasks (force-lock only).",
                    )
                startActivity(intent)
            }
        }

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

                1. Enter your server URL:
                   ws://<server-ip>:8848/device
                   (server machine + this phone on the same Wi-Fi)
                2. Tap Save & Connect.
                3. Enable "Abacad Probe" under Accessibility (button below);
                   accept the system warning.

                Once connected, an agent (via the server) can read the screen,
                inject taps, and screenshot this device.

                For hands-off, screen-off idle (optional):
                  • Set the screen lock to None or Swipe (NOT a PIN/pattern —
                    a secure lock cannot be auto-unlocked).
                  • Tap "Enable Screen Off" so `sleep` can turn the display off.
                  • Tap "Allow Display Over Other Apps" so `wake` can turn it back
                    on reliably on strict OEM ROMs.
                See docs/power-lockscreen.md for the full support matrix.

                Logs:  adb logcat -s ABACAD
            """.trimIndent()
        }

        root.addView(urlField)
        root.addView(connectBtn)
        root.addView(a11yBtn)
        root.addView(adminBtn)
        root.addView(overlayBtn)
        root.addView(info)
        setContentView(ScrollView(this).apply { addView(root) })
    }
}
