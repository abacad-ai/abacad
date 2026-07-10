import CoreGraphics
import Foundation
import ApplicationServices

// Synthesizes mouse and keyboard input via CGEvent. Coordinates are in the global
// top-left-origin point space — the same space AXPosition reports and the screen
// capture is scaled to, so a node's bounds map straight to a click point with no
// conversion. Posting requires the Accessibility permission (AXIsProcessTrusted).
enum InputInjection {
    private static let tap: CGEventTapLocation = .cghidEventTap

    static func flags(for names: [String]) -> CGEventFlags {
        var f: CGEventFlags = []
        for n in names { if let m = KeyMap.modifier(n) { f.insert(m) } }
        return f
    }

    // MARK: Mouse

    static func click(x: Int, y: Int, button: CGMouseButton = .left,
                      count: Int = 1, modifiers: [String] = []) {
        let pt = CGPoint(x: x, y: y)
        let f = flags(for: modifiers)
        let (down, up): (CGEventType, CGEventType) = button == .right
            ? (.rightMouseDown, .rightMouseUp)
            : (.leftMouseDown, .leftMouseUp)
        for i in 1...max(1, count) {
            post(mouse: down, pt: pt, button: button, flags: f, clickState: i)
            post(mouse: up, pt: pt, button: button, flags: f, clickState: i)
        }
    }

    static func rightClick(x: Int, y: Int) {
        click(x: x, y: y, button: .right)
    }

    /// Press at start, hold for `holdMs`, release — a press-and-hold at one point.
    static func longPress(x: Int, y: Int, holdMs: Int) {
        let pt = CGPoint(x: x, y: y)
        post(mouse: .leftMouseDown, pt: pt, button: .left, flags: [], clickState: 1)
        usleep(useconds_t(max(0, holdMs) * 1000))
        post(mouse: .leftMouseUp, pt: pt, button: .left, flags: [], clickState: 1)
    }

    /// Press at (x1,y1), interpolate to (x2,y2) over durationMs, release.
    static func drag(x1: Int, y1: Int, x2: Int, y2: Int, durationMs: Int, modifiers: [String] = []) {
        let f = flags(for: modifiers)
        let start = CGPoint(x: x1, y: y1)
        let end = CGPoint(x: x2, y: y2)
        post(mouse: .leftMouseDown, pt: start, button: .left, flags: f, clickState: 1)
        let steps = max(1, min(60, durationMs / 8))
        let perStep = UInt32(max(0, durationMs) * 1000 / steps)
        for i in 1...steps {
            let t = Double(i) / Double(steps)
            let p = CGPoint(x: x1 + Int(Double(x2 - x1) * t), y: y1 + Int(Double(y2 - y1) * t))
            post(mouse: .leftMouseDragged, pt: p, button: .left, flags: f, clickState: 1)
            if perStep > 0 { usleep(perStep) }
        }
        post(mouse: .leftMouseUp, pt: end, button: .left, flags: f, clickState: 1)
    }

    /// Scroll by a wheel delta at a point. Positive dy scrolls content up.
    static func scroll(x: Int, y: Int, dx: Int, dy: Int) {
        // Move the cursor to the target first: scroll events land at the current
        // pointer location, they carry no coordinates of their own.
        post(mouse: .mouseMoved, pt: CGPoint(x: x, y: y), button: .left, flags: [], clickState: 0)
        if let e = CGEvent(scrollWheelEvent2Source: nil, units: .line, wheelCount: 2,
                           wheel1: Int32(dy), wheel2: Int32(dx), wheel3: 0) {
            e.post(tap: tap)
        }
    }

    private static func post(mouse type: CGEventType, pt: CGPoint, button: CGMouseButton,
                             flags: CGEventFlags, clickState: Int) {
        guard let e = CGEvent(mouseEventSource: nil, mouseType: type,
                              mouseCursorPosition: pt, mouseButton: button) else { return }
        if !flags.isEmpty { e.flags = flags }
        if clickState > 1 { e.setIntegerValueField(.mouseEventClickState, value: Int64(clickState)) }
        e.post(tap: tap)
    }

    // MARK: Keyboard

    /// Press a chord: modifiers held while the main key(s) go down then up.
    /// Returns false if no main (non-modifier) key was recognized.
    @discardableResult
    static func pressChord(_ keys: [String]) -> Bool {
        var f: CGEventFlags = []
        var mains: [CGKeyCode] = []
        for k in keys {
            if let m = KeyMap.modifier(k) { f.insert(m) }
            else if let kc = KeyMap.keyCode(k) { mains.append(kc) }
        }
        guard !mains.isEmpty else { return false }
        let src = CGEventSource(stateID: .hidSystemState)
        for kc in mains { postKey(src, kc, down: true, flags: f) }
        for kc in mains.reversed() { postKey(src, kc, down: false, flags: f) }
        return true
    }

    /// Type a Unicode string as keystrokes (used as an input_text fallback).
    static func typeText(_ text: String) {
        let src = CGEventSource(stateID: .hidSystemState)
        for scalar in text.unicodeScalars {
            var ch = UniChar(scalar.value & 0xFFFF)
            guard let down = CGEvent(keyboardEventSource: src, virtualKey: 0, keyDown: true),
                  let up = CGEvent(keyboardEventSource: src, virtualKey: 0, keyDown: false) else { continue }
            down.keyboardSetUnicodeString(stringLength: 1, unicodeString: &ch)
            up.keyboardSetUnicodeString(stringLength: 1, unicodeString: &ch)
            down.post(tap: tap)
            up.post(tap: tap)
        }
    }

    private static func postKey(_ src: CGEventSource?, _ kc: CGKeyCode, down: Bool, flags: CGEventFlags) {
        guard let e = CGEvent(keyboardEventSource: src, virtualKey: kc, keyDown: down) else { return }
        if !flags.isEmpty { e.flags = flags }
        e.post(tap: tap)
    }
}
