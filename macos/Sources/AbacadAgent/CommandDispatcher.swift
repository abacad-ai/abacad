import Foundation
import CoreGraphics
import ApplicationServices

// Routes a parsed {id, method, params} command to a handler and produces the
// {id, ok, result|error} reply. Correlation is purely by id; replies may be sent
// out of order (screenshots are async). Malformed frames are dropped upstream
// with no reply, matching the Android client.
//
// The Mac answers a superset: the mobile verbs (mapped to desktop equivalents so
// today's tools/agent work unchanged) plus the desktop-native verbs. Anything
// unrecognized returns "unknown method: X" — which is how the server keeps one
// global tool list without per-platform filtering.
struct CommandDispatcher {
    /// Execute a method and return the `result` object, or throw CmdError.
    func execute(method: String, params: [String: Any]) async throws -> [String: Any] {
        switch method {
        case "screenshot":
            return try await screenshot(includeTree: params.bool("include_ui_tree", true))

        // Mobile verbs, mapped onto desktop input for cross-platform compatibility.
        case "tap":
            requireCoords(params, "x", "y")
            InputInjection.click(x: params.int("x"), y: params.int("y"))
            return ["dispatched": true]
        case "long_press":
            requireCoords(params, "x", "y")
            InputInjection.longPress(x: params.int("x"), y: params.int("y"), holdMs: params.int("duration_ms", 600))
            return ["dispatched": true]
        case "swipe":
            InputInjection.drag(x1: params.int("x1"), y1: params.int("y1"),
                                x2: params.int("x2"), y2: params.int("y2"),
                                durationMs: params.int("duration_ms", 300))
            return ["dispatched": true]
        case "input_text":
            return ["set": setFocusedText(params.string("text"))]

        // Desktop-native verbs.
        case "click":
            requireCoords(params, "x", "y")
            InputInjection.click(x: params.int("x"), y: params.int("y"),
                                 button: .left, count: params.int("count", 1),
                                 modifiers: params.strings("modifiers"))
            return ["dispatched": true]
        case "right_click":
            requireCoords(params, "x", "y")
            InputInjection.rightClick(x: params.int("x"), y: params.int("y"))
            return ["dispatched": true]
        case "drag":
            InputInjection.drag(x1: params.int("x1"), y1: params.int("y1"),
                                x2: params.int("x2"), y2: params.int("y2"),
                                durationMs: params.int("duration_ms", 300),
                                modifiers: params.strings("modifiers"))
            return ["dispatched": true]
        case "scroll":
            requireCoords(params, "x", "y")
            InputInjection.scroll(x: params.int("x"), y: params.int("y"),
                                  dx: params.int("dx"), dy: params.int("dy"))
            return ["dispatched": true]
        case "press_keys":
            let keys = params.strings("keys")
            guard !keys.isEmpty else { throw CmdError.message("press_keys requires a non-empty keys array") }
            let pressed = InputInjection.pressChord(keys)
            if !pressed { throw CmdError.message("press_keys: no recognized key in \(keys)") }
            return ["pressed": true]
        case "composite":
            let steps = params.objects("steps")
            guard !steps.isEmpty else { throw CmdError.message("composite requires a non-empty steps array") }
            return try await Composite.run(steps)

        // Mobile navigation keys have no desktop analogue.
        case "back", "home", "recents":
            throw CmdError.message("\(method) has no desktop analogue — use click / press_keys")

        default:
            throw CmdError.message("unknown method: \(method)")
        }
    }

    // MARK: Handlers

    private func screenshot(includeTree: Bool) async throws -> [String: Any] {
        let shot = try await ScreenCapture.capture()
        var result: [String: Any] = ["w": shot.w, "h": shot.h, "png_base64": shot.base64]
        if includeTree, let tree = AccessibilityTree.capture() { result["tree"] = tree }
        return result
    }

    /// Replace the focused field's contents via AX (matches Android's input_text
    /// "set text" semantics). Falls back to typing if the element rejects the set.
    private func setFocusedText(_ text: String) -> Bool {
        let system = AXUIElementCreateSystemWide()
        var focused: CFTypeRef?
        guard AXUIElementCopyAttributeValue(system, kAXFocusedUIElementAttribute as CFString, &focused) == .success,
              let element = focused else {
            InputInjection.typeText(text)
            return true
        }
        let el = element as! AXUIElement
        let err = AXUIElementSetAttributeValue(el, kAXValueAttribute as CFString, text as CFString)
        if err != .success {
            InputInjection.typeText(text)
        }
        return true
    }

    private func requireCoords(_ p: [String: Any], _ keys: String...) {
        // Loose: missing keys default to 0. Kept as a hook for future validation;
        // desktop coords can legitimately be 0, so we don't reject negatives the
        // way the phone does.
        _ = keys
    }
}
