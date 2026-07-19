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

    private fun append(text: String) {
        val snapshot: List<() -> Unit>
        synchronized(this) {
            lines.addLast(Line(System.currentTimeMillis(), text))
            while (lines.size > MAX_LINES) lines.removeFirst()
            snapshot = listeners.toList()
        }
        snapshot.forEach { it() }
    }
}
