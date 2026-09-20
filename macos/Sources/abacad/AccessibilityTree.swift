import ApplicationServices
import AppKit

// Walks the accessibility tree of what is on screen and emits the same flat
// shape the Android client produces, so the server's UITree decoding and the
// agent's reasoning are identical across platforms:
//   { "pkg": <bundle id>, "nodes": [ {cls, text, id, clickable, bounds:[l,t,r,b]} ] }
//
// Bounds are AXPosition/AXSize in global top-left points — the same space
// InputInjection clicks in — so a node's bounds map directly to a click point.
//
// macOS scopes an accessibility tree to one process, and the screen is not one
// process. Walking only the frontmost application described a plain Finder
// desktop as 343 nodes that left out eleven Dock tiles, the entire right-hand
// menu bar and three desktop widgets — all of them plainly visible, none of them
// reachable by an agent that can act only on what the tree reports. So the
// frontmost application is walked first and the processes that own the rest of
// the furniture are walked after it.
enum AccessibilityTree {
    private static let maxNodes = 3000       // matches the Android BFS cap
    private static let maxNodesPerApp = 400   // the Dock's or the widget host's share
    private static let maxNodesPerExtra = 40  // one status item is a handful of nodes

    // Furniture that is always on screen and always owned by the same process,
    // so it can be named outright.
    private static let furniture: Set<String> = [
        "com.apple.dock",    // the Dock, and Mission Control's tiles
        "com.apple.chronod", // desktop and Notification Centre widgets
    ]

    static func capture() -> [String: Any]? {
        guard let front = NSWorkspace.shared.frontmostApplication else { return nil }
        let pkg = front.bundleIdentifier ?? (front.localizedName ?? "")

        // The frontmost application goes first and may use the whole budget: its
        // window is what a task is usually about, and a dense one must not lose
        // detail to the Dock.
        var nodes: [[String: Any]] = []
        walk(from: AXUIElementCreateApplication(front.processIdentifier),
             into: &nodes, limit: maxNodes)

        for app in NSWorkspace.shared.runningApplications {
            guard nodes.count < maxNodes,
                  app.processIdentifier != front.processIdentifier,
                  app.activationPolicy != .prohibited else { continue }
            if let id = app.bundleIdentifier, furniture.contains(id) {
                walk(from: AXUIElementCreateApplication(app.processIdentifier),
                     into: &nodes, limit: min(maxNodesPerApp, maxNodes - nodes.count))
                continue
            }
            walkExtras(pid: app.processIdentifier, into: &nodes,
                       limit: min(maxNodesPerExtra, maxNodes - nodes.count))
        }
        return ["pkg": pkg, "nodes": nodes]
    }

    /// Walk a process's menu-bar extra, if it publishes one.
    ///
    /// The right-hand end of the menu bar cannot be named the way the Dock can,
    /// because there is no process that owns it: Wi-Fi, the input-source picker
    /// and the battery are each their own agent, and every third-party status
    /// item belongs to whichever app installed it — measured at fourteen
    /// separate processes on one machine. What they share is the attribute, so
    /// asking each running application for its AXExtrasMenuBar finds all of them
    /// and stays out of every window those applications also happen to own.
    private static func walkExtras(pid: pid_t, into nodes: inout [[String: Any]], limit: Int) {
        guard limit > 0 else { return }
        let root = AXUIElementCreateApplication(pid)
        AXUIElementSetMessagingTimeout(root, 0.25)
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(root, kAXExtrasMenuBarAttribute as CFString,
                                            &value) == .success,
              let bar = value, CFGetTypeID(bar) == AXUIElementGetTypeID() else { return }
        walk(from: bar as! AXUIElement, into: &nodes, limit: limit)
    }

    /// Breadth-first walk from one element, appending at most `limit` nodes.
    ///
    /// The messaging timeout is what makes this safe to do for a process we did
    /// not choose: an AX request to a wedged application blocks its caller by
    /// default, and a screenshot must not wait on somebody's status item.
    private static func walk(from root: AXUIElement, into nodes: inout [[String: Any]], limit: Int) {
        guard limit > 0 else { return }
        AXUIElementSetMessagingTimeout(root, 0.5)

        let start = nodes.count
        var queue: [AXUIElement] = [root]
        var i = 0
        while i < queue.count && nodes.count - start < limit {
            let el = queue[i]; i += 1
            if let node = describe(el) { nodes.append(node) }
            for child in children(el) {
                if queue.count >= limit { break }
                queue.append(child)
            }
        }
    }

    private static func describe(_ el: AXUIElement) -> [String: Any]? {
        let role = stringAttr(el, kAXRoleAttribute) ?? ""
        // Prefer visible text: value, then title, then description.
        let text = stringAttr(el, kAXValueAttribute)
            ?? stringAttr(el, kAXTitleAttribute)
            ?? stringAttr(el, kAXDescriptionAttribute)
            ?? ""
        let id = stringAttr(el, kAXIdentifierAttribute) ?? ""
        let clickable = actionable(el)
        let bounds = frame(el)
        // Skip the application root and any node with neither text nor a frame.
        if role.isEmpty && text.isEmpty && bounds == nil { return nil }
        return [
            "cls": role,
            "text": text,
            "id": id,
            "clickable": clickable,
            "bounds": bounds ?? [0, 0, 0, 0],
        ]
    }

    private static func children(_ el: AXUIElement) -> [AXUIElement] {
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(el, kAXChildrenAttribute as CFString, &value) == .success,
              let arr = value as? [AXUIElement] else { return [] }
        return arr
    }

    private static func actionable(_ el: AXUIElement) -> Bool {
        var names: CFArray?
        guard AXUIElementCopyActionNames(el, &names) == .success,
              let actions = names as? [String] else { return false }
        return actions.contains(kAXPressAction as String)
    }

    private static func frame(_ el: AXUIElement) -> [Int]? {
        guard let pos = axValue(el, kAXPositionAttribute, .cgPoint) as CGPoint?,
              let size = axValue(el, kAXSizeAttribute, .cgSize) as CGSize? else { return nil }
        let l = Int(pos.x), t = Int(pos.y)
        return [l, t, l + Int(size.width), t + Int(size.height)]
    }

    private static func stringAttr(_ el: AXUIElement, _ attr: String) -> String? {
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(el, attr as CFString, &value) == .success else { return nil }
        if let s = value as? String { return s.isEmpty ? nil : s }
        if let n = value as? NSNumber { return n.stringValue }
        return nil
    }

    // Decode an AXValue-wrapped CGPoint / CGSize.
    private static func axValue<T>(_ el: AXUIElement, _ attr: String, _ type: AXValueType) -> T? {
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(el, attr as CFString, &value) == .success,
              let axv = value, CFGetTypeID(axv) == AXValueGetTypeID() else { return nil }
        let av = axv as! AXValue
        if type == .cgPoint {
            var p = CGPoint.zero
            if AXValueGetValue(av, .cgPoint, &p) { return p as? T }
        } else if type == .cgSize {
            var s = CGSize.zero
            if AXValueGetValue(av, .cgSize, &s) { return s as? T }
        }
        return nil
    }
}
