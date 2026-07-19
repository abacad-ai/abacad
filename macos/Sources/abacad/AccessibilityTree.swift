import ApplicationServices
import AppKit

// Walks the accessibility tree of the frontmost application and emits the same
// flat shape the Android client produces, so the server's UITree decoding and the
// agent's reasoning are identical across platforms:
//   { "pkg": <bundle id>, "nodes": [ {cls, text, id, clickable, bounds:[l,t,r,b]} ] }
//
// Bounds are AXPosition/AXSize in global top-left points — the same space
// InputInjection clicks in — so a node's bounds map directly to a click point.
enum AccessibilityTree {
    private static let maxNodes = 3000 // matches the Android BFS cap

    static func capture() -> [String: Any]? {
        guard let app = NSWorkspace.shared.frontmostApplication else { return nil }
        let pkg = app.bundleIdentifier ?? (app.localizedName ?? "")
        let root = AXUIElementCreateApplication(app.processIdentifier)

        var nodes: [[String: Any]] = []
        var queue: [AXUIElement] = [root]
        var i = 0
        while i < queue.count && nodes.count < maxNodes {
            let el = queue[i]; i += 1
            if let node = describe(el) { nodes.append(node) }
            for child in children(el) {
                if queue.count >= maxNodes { break }
                queue.append(child)
            }
        }
        return ["pkg": pkg, "nodes": nodes]
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
