package dev.abacad.probe

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.provider.Settings
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView

/**
 * Deliberately minimal: one button to jump to Accessibility settings, plus
 * on-screen instructions. All actual probe RESULTS go to Logcat (tag
 * ABACAD_PROBE), not the UI — this screen only exists to enable the service.
 */
class MainActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val pad = (24 * resources.displayMetrics.density).toInt()
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }

        val button = Button(this).apply {
            text = "Open Accessibility Settings"
            setOnClickListener {
                startActivity(Intent(Settings.ACTION_ACCESSIBILITY_SETTINGS))
            }
        }

        val info = TextView(this).apply {
            textSize = 13f
            text = """
                Abacad — capability probe (throwaway)

                Setup
                  1. Tap the button above.
                  2. Enable "Abacad Probe" under Accessibility.
                     (accept the system permission warning)
                  3. Probe runs immediately, then again each time
                     you open a new app/screen.

                Read results over USB
                  adb logcat -s ABACAD_PROBE

                Re-run manually
                  adb shell am broadcast -a dev.abacad.probe.RUN

                Pull the captured screenshot
                  adb pull /sdcard/Android/data/dev.abacad.probe/files/probe_shot.png

                PASS = tree has real nodes, SHOT SUCCESS + nonBlack=true,
                TAP onCompleted — and NO screen-capture consent dialog
                ever appeared.
            """.trimIndent()
        }

        root.addView(button)
        root.addView(info)
        setContentView(ScrollView(this).apply { addView(root) })
    }
}
