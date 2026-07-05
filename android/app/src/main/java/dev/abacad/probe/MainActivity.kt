package dev.abacad.probe

import android.app.Activity
import android.content.Context
import android.content.Intent
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

                Logs:  adb logcat -s ABACAD
            """.trimIndent()
        }

        root.addView(urlField)
        root.addView(connectBtn)
        root.addView(a11yBtn)
        root.addView(info)
        setContentView(ScrollView(this).apply { addView(root) })
    }
}
