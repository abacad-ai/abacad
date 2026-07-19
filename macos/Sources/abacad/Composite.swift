import CoreGraphics
import Foundation

// Executes an ordered `composite` step list on-device with real timing — the
// low-level primitive the named verbs are sugar over. Steps carry an "op":
//
//   pointer_down {x,y,button?}   pointer_move {x,y}   pointer_up {button?}
//   key_down {key}   key_up {key}   type {text}
//   click {x,y,button?,count?,modifiers?}   wait {ms}   screenshot {}
//
// Any screenshot steps return their frames in order under {"shots": [...]}.
// Single-pointer only — macOS has no public multi-touch injection.
enum Composite {
    static func run(_ steps: [[String: Any]]) async throws -> [String: Any] {
        var shots: [[String: Any]] = []
        let src = CGEventSource(stateID: .hidSystemState)
        var buttonDown = false
        var lastPoint = CGPoint.zero

        for step in steps {
            let op = step.string("op")
            switch op {
            case "pointer_down":
                lastPoint = CGPoint(x: step.int("x"), y: step.int("y"))
                let button = mouseButton(step.string("button", "left"))
                postMouse(button == .right ? .rightMouseDown : .leftMouseDown, lastPoint, button)
                buttonDown = true
            case "pointer_move":
                lastPoint = CGPoint(x: step.int("x"), y: step.int("y"))
                postMouse(buttonDown ? .leftMouseDragged : .mouseMoved, lastPoint, .left)
            case "pointer_up":
                let button = mouseButton(step.string("button", "left"))
                postMouse(button == .right ? .rightMouseUp : .leftMouseUp, lastPoint, button)
                buttonDown = false
            case "key_down":
                if let kc = KeyMap.keyCode(step.string("key")) { key(src, kc, down: true) }
            case "key_up":
                if let kc = KeyMap.keyCode(step.string("key")) { key(src, kc, down: false) }
            case "type":
                InputInjection.typeText(step.string("text"))
            case "click":
                InputInjection.click(x: step.int("x"), y: step.int("y"),
                                     button: mouseButton(step.string("button", "left")),
                                     count: step.int("count", 1),
                                     modifiers: step.strings("modifiers"))
            case "wait":
                let ms = step.int("ms")
                if ms > 0 { try? await Task.sleep(nanoseconds: UInt64(ms) * 1_000_000) }
            case "screenshot":
                let shot = try await ScreenCapture.capture()
                shots.append(["w": shot.w, "h": shot.h, "png_base64": shot.base64])
            default:
                throw CmdError.message("composite: unknown op \"\(op)\"")
            }
        }
        return ["shots": shots]
    }

    private static func mouseButton(_ name: String) -> CGMouseButton {
        name.lowercased() == "right" ? .right : .left
    }

    private static func postMouse(_ type: CGEventType, _ pt: CGPoint, _ button: CGMouseButton) {
        guard let e = CGEvent(mouseEventSource: nil, mouseType: type,
                              mouseCursorPosition: pt, mouseButton: button) else { return }
        e.post(tap: .cghidEventTap)
    }

    private static func key(_ src: CGEventSource?, _ kc: CGKeyCode, down: Bool) {
        guard let e = CGEvent(keyboardEventSource: src, virtualKey: kc, keyDown: down) else { return }
        e.post(tap: .cghidEventTap)
    }
}
