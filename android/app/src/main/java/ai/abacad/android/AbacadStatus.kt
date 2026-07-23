package ai.abacad.android

/**
 * In-process connection + activity status, shared between the accessibility
 * service (the writer) and [MainActivity] (the reader). It is the single source
 * of truth for the in-app status panel; every write is also mirrored to logcat
 * by its caller, so `adb logcat -s ABACAD` and the on-screen panel agree.
 *
 * There is no persistence and no cross-process concern — the service and the UI
 * run in the same process, so a plain singleton with a small synchronized ring
 * buffer is enough. Listeners are notified on whatever thread writes; the UI
 * listener marshals to the main thread itself.
 */
object AbacadStatus {

    /** Coarse connection state, drives the panel's headline + color. */
    enum class State { DISCONNECTED, CONNECTING, CONNECTED, RECONNECTING }

    /** One timestamped activity line (a state change, a command, an error). */
    data class Line(val ts: Long, val text: String)

    @Volatile
    var state: State = State.DISCONNECTED
        private set

    /** Human detail for the current [state], e.g. "connected" or "reconnecting in 4000ms". */
    @Volatile
    var detail: String = "not connected"
        private set

    /**
     * Soft-kill: the device operator has paused control. While true the client
     * stays connected but [DeviceClient] rejects every incoming command locally.
     * Cleared only from the app (never by the agent), so it's a real local stop.
     */
    @Volatile
    var paused: Boolean = false
        private set

    /** A live-view (VNC) session is active — someone is watching this screen right now. */
    @Volatile
    var watched: Boolean = false
        private set

    /** A screen recording is in progress. */
    @Volatile
    var recording: Boolean = false
        private set

    /** Wall-clock of the last command the agent ran, and its method — drives the
     *  "controlling now" state and the header subtitle. */
    @Volatile
    var lastCommandAt: Long = 0
        private set

    @Volatile
    var lastMethod: String? = null
        private set

    private const val MAX_LINES = 40
    private val lines = ArrayDeque<Line>()
    private val listeners = LinkedHashSet<() -> Unit>()

    /** Recent activity, oldest first. */
    @Synchronized
    fun recentLines(): List<Line> = lines.toList()

    @Synchronized
    fun addListener(l: () -> Unit) { listeners.add(l) }

    @Synchronized
    fun removeListener(l: () -> Unit) { listeners.remove(l) }

    /** Move to [s] with a one-line [detail]; recorded as an activity line too. */
    fun setState(s: State, detail: String) {
        state = s
        this.detail = detail
        append("• $detail")
    }

    /** Record a discrete activity line (a command outcome, a diagnostic) without changing state. */
    fun event(text: String) {
        append(text)
    }

    /** Toggle the soft-kill pause (from the app). Recorded as an activity line. */
    fun setPaused(p: Boolean) {
        if (paused == p) return
        paused = p
        append(if (p) "⏸ control paused by device operator" else "▶ control resumed")
    }

    /** Live-view (watched) flag; [DeviceClient] sets it from vnc start/stop. */
    fun setWatched(w: Boolean) {
        if (watched == w) return
        watched = w
        append(if (w) "👁 live view started — screen being watched" else "live view ended")
    }

    /** Recording flag; set from screen_recording start/stop. */
    fun setRecording(r: Boolean) {
        if (recording == r) return
        recording = r
        append(if (r) "● screen recording started" else "screen recording stopped")
    }

    /** Note that a command arrived (drives the "controlling now" state). */
    fun noteCommand(method: String) {
        lastCommandAt = System.currentTimeMillis()
        lastMethod = method
        notifyListeners()
    }

    /**
     * True when an agent is actively driving right now: connected, not paused, and
     * a command ran within the last [WINDOW_MS]. The UI shows a distinct
     * "Controlling now" state vs. a plain idle "Connected".
     */
    fun controlling(): Boolean =
        state == State.CONNECTED &&
            !paused &&
            System.currentTimeMillis() - lastCommandAt < CONTROLLING_WINDOW_MS

    private const val CONTROLLING_WINDOW_MS = 6_000L

    private fun append(text: String) {
        synchronized(this) {
            lines.addLast(Line(System.currentTimeMillis(), text))
            while (lines.size > MAX_LINES) lines.removeFirst()
        }
        notifyListeners()
    }

    private fun notifyListeners() {
        val snapshot: List<() -> Unit>
        synchronized(this) { snapshot = listeners.toList() }
        snapshot.forEach { it() }
    }
}
