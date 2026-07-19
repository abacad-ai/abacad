import CoreGraphics

// Maps friendly key names (as an agent would send them) to macOS virtual key
// codes and modifier flags. US layout — documented limitation; a non-US layout
// would need a UCKeyTranslate-based lookup, out of scope for v0.
enum KeyMap {
    /// Returns the CGEventFlags mask if `name` is a modifier, else nil.
    static func modifier(_ name: String) -> CGEventFlags? {
        switch name.lowercased() {
        case "cmd", "command", "meta", "super": return .maskCommand
        case "shift": return .maskShift
        case "opt", "option", "alt": return .maskAlternate
        case "ctrl", "control": return .maskControl
        case "fn", "function": return .maskSecondaryFn
        default: return nil
        }
    }

    /// Returns the virtual key code for a non-modifier key name, else nil.
    static func keyCode(_ name: String) -> CGKeyCode? {
        let n = name.lowercased()
        if let named = named[n] { return named }
        if n.count == 1, let c = n.first, let code = chars[c] { return code }
        return nil
    }

    private static let named: [String: CGKeyCode] = [
        "enter": 36, "return": 36, "tab": 48, "space": 49, "delete": 51,
        "backspace": 51, "esc": 53, "escape": 53, "forwarddelete": 117,
        "left": 123, "right": 124, "down": 125, "up": 126,
        "home": 115, "end": 119, "pageup": 116, "pagedown": 121,
        "f1": 122, "f2": 120, "f3": 99, "f4": 118, "f5": 96, "f6": 97,
        "f7": 98, "f8": 100, "f9": 101, "f10": 109, "f11": 103, "f12": 111,
    ]

    // US-layout character → keycode. Letters, digits, and common punctuation.
    private static let chars: [Character: CGKeyCode] = [
        "a": 0, "s": 1, "d": 2, "f": 3, "h": 4, "g": 5, "z": 6, "x": 7, "c": 8,
        "v": 9, "b": 11, "q": 12, "w": 13, "e": 14, "r": 15, "y": 16, "t": 17,
        "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "9": 25,
        "7": 26, "-": 27, "8": 28, "0": 29, "]": 30, "o": 31, "u": 32, "[": 33,
        "i": 34, "p": 35, "l": 37, "j": 38, "'": 39, "k": 40, ";": 41, "\\": 42,
        ",": 43, "/": 44, "n": 45, "m": 46, ".": 47, "`": 50,
    ]
}
