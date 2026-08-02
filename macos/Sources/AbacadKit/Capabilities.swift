import Foundation

/// The device-side capability ceiling: which interfaces this Mac is willing to
/// expose, decided here rather than by the relay.
///
/// The server keeps its own per-device set and the effective surface is the
/// intersection. The difference between the two is who you have to trust. The
/// server's set protects you from a misbehaving agent; this one protects you
/// from a misbehaving *server* — a relay that is compromised, out of date, or
/// simply run by somebody else cannot switch a capability back on here, because
/// the agent refuses the command regardless of what it is told.
///
/// That is why the check sits in front of the dispatcher rather than being
/// trusted to the wire. In normal operation it never fires, because the relay
/// already declined to send — and that redundancy is the point, not waste.
///
/// Honest limit: this cannot constrain a capability that already grants code
/// execution as this user. Turning file transfer off is meaningful; leaving it
/// on and expecting the rest to hold is not.
public enum Capabilities {

    /// The vocabulary, mirroring the server's protocol.Capabilities.
    public static let all = [
        "screenshot", "tap", "long_press", "swipe", "input_text",
        "back", "home", "recents",
        "click", "right_click", "drag", "scroll", "press_keys", "composite",
        "execute", "push_file", "pull_file", "screen_recording",
        "tunnel", "ssh", "vnc",
    ]

    /// "Everything, including capabilities added in later versions." An
    /// unconfigured Mac reports this rather than enumerating `all`, so it does
    /// not pin itself to the verb list of the version it shipped with.
    public static let wildcard = "*"

    /// Stored sentinel for "expose nothing".
    ///
    /// Needed because Prefs.keychainGet returns nil for an empty string, so ""
    /// and "no item at all" are indistinguishable there — and those two mean
    /// opposite things here: no config is the wildcard, an empty set is a
    /// refusal of everything. Without an explicit marker the most restrictive
    /// setting would silently read back as the most permissive one.
    private static let emptyMarker = "none"

    private static let lock = NSLock()
    private static var wildcardMode = true // no config yet => expose everything
    private static var enabled: Set<String> = []
    private static var listeners: [() -> Void] = []

    /// Reads the persisted set. Call before connecting: the ceiling must be in
    /// force before the first command can arrive, and it is reported on connect.
    public static func load() {
        let raw = Prefs.capabilities
        lock.lock()
        defer { lock.unlock() }
        if raw.isEmpty { return } // never configured => wildcard
        applyLocked(raw.split(separator: ",").map {
            $0.trimmingCharacters(in: .whitespaces)
        }.filter { !$0.isEmpty })
    }

    /// Whether this device exposes `name`.
    public static func allows(_ name: String) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return wildcardMode || enabled.contains(name)
    }

    /// What this device advertises to the server: the full set, always, so the
    /// latest frame is the whole truth and no delta can drift. The wildcard
    /// stays a wildcard rather than being expanded, so a newer server keeps
    /// granting capabilities this build has never heard of.
    public static func report() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        if wildcardMode { return [wildcard] }
        return all.filter { enabled.contains($0) }
    }

    /// The concrete set, expanding the wildcard, for rendering checkboxes.
    public static func enabledList() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        return all.filter { wildcardMode || enabled.contains($0) }
    }

    /// Turns one capability on or off, persists, and notifies listeners.
    /// Switching one off while in wildcard mode first materializes the wildcard
    /// into the concrete set, so "everything except X" is expressible.
    public static func toggle(_ name: String, on: Bool) {
        lock.lock()
        if wildcardMode {
            wildcardMode = false
            enabled = Set(all)
        }
        if on { enabled.insert(name) } else { enabled.remove(name) }
        let subs = listeners
        saveLocked()
        lock.unlock()
        subs.forEach { $0() }
    }

    /// Replaces the whole set.
    public static func set(_ names: [String]) {
        lock.lock()
        applyLocked(names)
        let subs = listeners
        saveLocked()
        lock.unlock()
        subs.forEach { $0() }
    }

    /// Registers a change callback (the agent re-reports; the UI repaints).
    public static func onChange(_ fn: @escaping () -> Void) {
        lock.lock()
        listeners.append(fn)
        lock.unlock()
    }

    // MARK: - Command authorization

    /// Composite step op -> the capabilities it needs.
    ///
    /// composite is the one verb that performs other verbs, so authorizing only
    /// the outer method would let a permitted composite do everything the
    /// ceiling forbids. Covers ops this build does not implement as well: the
    /// table is a policy statement, and being stricter than the executor costs
    /// nothing while being looser would be a hole.
    private static let compositeOps: [String: [String]] = [
        "pointer_down": ["click", "drag"],
        "pointer_move": ["click", "drag"],
        "pointer_up": ["click", "drag"],
        "click": ["click"],
        "tap": ["tap"],
        "long_press": ["long_press"],
        "swipe": ["swipe"],
        "drag": ["drag"],
        "scroll": ["scroll"],
        "key_down": ["press_keys"],
        "key_up": ["press_keys"],
        "type": ["input_text"],
        "screenshot": ["screenshot"],
        "wait": [],
    ]

    /// Authorizes one inbound command. Returns nil when allowed, or the reason
    /// to refuse with.
    public static func refusal(method: String, params: [String: Any]) -> String? {
        // Stopping is never blocked. vnc and screen_recording multiplex
        // start/stop through one method, so refusing the stop would strand the
        // very session the operator just revoked — the screen would stay shared
        // with no way to end it short of quitting. A ceiling prevents things
        // starting, never ceasing.
        if method == "vnc" || method == "screen_recording",
           (params["action"] as? String) == "stop" {
            return nil
        }
        if !allows(method) {
            return "\(method) is turned off on this device — re-enable it in the abacad menu"
        }
        guard method == "composite" else { return nil }
        guard let steps = params["steps"] as? [[String: Any]] else { return nil }
        for (i, step) in steps.enumerated() {
            guard let op = step["op"] as? String else {
                return "composite: step \(i) has no op"
            }
            // Unknown ops are REFUSED, not waved through: a newer server must
            // not be able to slip an unmapped action past a ceiling this build
            // predates.
            guard let needed = compositeOps[op] else {
                return "composite: step \(i) has unknown op \"\(op)\""
            }
            for need in needed where !allows(need) {
                return "composite: step \(i) (\(op)): \(need) is turned off on this device"
            }
        }
        return nil
    }

    /// Authorizes a tunnel dial.
    ///
    /// The device sees only a host:port, not which server-side consumer asked,
    /// so it cannot distinguish the SSH jump from a plain /connect the way the
    /// relay can. It infers instead: the jump always targets this machine's own
    /// sshd, so a loopback :22 dial is allowed by EITHER ssh or tunnel, and
    /// anything else needs tunnel. That mirrors the real relationship — a tunnel
    /// can reach port 22 on its own, so treating them as independent would be a
    /// fiction.
    public static func tunnelRefusal(host: String, port: String) -> String? {
        let loopback = host == "localhost" || host == "127.0.0.1" || host == "::1"
        if port == "22" && loopback {
            return (allows("ssh") || allows("tunnel"))
                ? nil : "ssh is turned off on this device"
        }
        return allows("tunnel") ? nil : "tunnel is turned off on this device"
    }

    // MARK: - Private

    private static func applyLocked(_ names: [String]) {
        if names.contains(wildcard) {
            wildcardMode = true
            enabled = []
            return
        }
        wildcardMode = false
        enabled = Set(names.filter { $0 != emptyMarker })
    }

    private static func saveLocked() {
        if wildcardMode {
            Prefs.capabilities = wildcard
        } else {
            let on = all.filter { enabled.contains($0) }
            // See emptyMarker: an empty string would read back as "never
            // configured", turning "expose nothing" into "expose everything".
            Prefs.capabilities = on.isEmpty ? emptyMarker : on.joined(separator: ",")
        }
    }
}
