package ai.abacad.android

import android.content.Context

/**
 * The device-side capability ceiling: which interfaces this phone is willing to
 * expose, decided here rather than by the relay.
 *
 * The server keeps its own per-device set and the effective surface is the
 * intersection. The difference between the two is who you have to trust. The
 * server's set protects you from a misbehaving agent; this one protects you from
 * a misbehaving *server* — a relay that is compromised, out of date, or simply
 * run by somebody else cannot switch a capability back on here, because
 * [DeviceClient] refuses the command regardless of what it is told.
 *
 * That is why the check sits in front of the dispatcher instead of being trusted
 * to the wire. In normal operation it never fires, because the relay already
 * declined to send — and that redundancy is the point, not waste.
 *
 * Honest limit: this cannot constrain a capability that already grants code
 * execution as this app. Turning file transfer off is meaningful; leaving it on
 * and expecting the rest to hold is not.
 *
 * Unlike [AbacadStatus] this IS persisted — a ceiling that forgot itself on
 * restart would be worthless — but it follows the same plain-singleton shape.
 */
object AbacadCapabilities {

    /** The vocabulary, mirroring the server's protocol.Capabilities. */
    val ALL = listOf(
        "screenshot", "tap", "long_press", "swipe", "input_text",
        "back", "home", "recents",
        "click", "right_click", "drag", "scroll", "press_keys", "composite",
        "execute", "push_file", "pull_file", "screen_recording",
        "tunnel", "ssh", "vnc",
    )

    /**
     * "Everything, including capabilities added in later versions." An
     * unconfigured device reports this rather than enumerating [ALL], so it does
     * not pin itself to the verb list of the version it shipped with.
     */
    const val WILDCARD = "*"

    private const val KEY = "capabilities"

    @Volatile
    private var wildcard = true // no config yet => expose everything, as before

    @Volatile
    private var enabled: Set<String> = emptySet()

    private val listeners = mutableListOf<() -> Unit>()
    private var prefs: android.content.SharedPreferences? = null

    /**
     * Reads the persisted set. Call before connecting: the ceiling has to be in
     * force before the first command can arrive, and it is reported on connect.
     *
     * A missing key leaves the wildcard in place, so an existing install keeps
     * behaving exactly as it did before this feature existed.
     */
    @Synchronized
    fun load(context: Context) {
        val p = context.applicationContext
            .getSharedPreferences(AbacadAccessibilityService.PREFS, Context.MODE_PRIVATE)
        prefs = p
        val raw = p.getString(KEY, null) ?: return // absent => wildcard
        applyLocked(raw.split(",").map { it.trim() }.filter { it.isNotEmpty() })
    }

    /** Whether this device exposes [name]. */
    fun allows(name: String): Boolean = wildcard || enabled.contains(name)

    /**
     * What this device advertises to the server: the full set, always, so the
     * latest frame is the whole truth and no delta can drift. The wildcard stays
     * a wildcard rather than being expanded, so a newer server keeps granting
     * capabilities this build has never heard of.
     */
    fun report(): List<String> =
        if (wildcard) listOf(WILDCARD) else ALL.filter { enabled.contains(it) }

    /** The concrete set, expanding the wildcard, for rendering checkboxes. */
    fun enabledList(): List<String> = ALL.filter { allows(it) }

    /** Replaces the set, persists it, and notifies listeners. */
    @Synchronized
    fun set(names: List<String>) {
        applyLocked(names)
        save()
        notifyListeners()
    }

    /**
     * Turns one capability on or off. Switching one off while in wildcard mode
     * first materializes the wildcard into the concrete set, so "everything
     * except X" is expressible.
     */
    @Synchronized
    fun toggle(name: String, on: Boolean) {
        val next = (if (wildcard) ALL.toMutableSet() else enabled.toMutableSet())
        if (on) next.add(name) else next.remove(name)
        wildcard = false
        enabled = next
        save()
        notifyListeners()
    }

    /** Registers a change callback (the client re-reports; the UI repaints). */
    @Synchronized
    fun onChange(fn: () -> Unit) {
        listeners.add(fn)
    }

    private fun applyLocked(names: List<String>) {
        if (names.contains(WILDCARD)) {
            wildcard = true
            enabled = emptySet()
            return
        }
        // An empty list is meaningful: expose nothing. Distinct from no key at
        // all, which means the wildcard.
        wildcard = false
        enabled = names.toSet()
    }

    private fun save() {
        val value = if (wildcard) WILDCARD else ALL.filter { enabled.contains(it) }.joinToString(",")
        prefs?.edit()?.putString(KEY, value)?.apply()
    }

    private fun notifyListeners() {
        val subs = listeners.toList()
        for (fn in subs) fn()
    }
}
