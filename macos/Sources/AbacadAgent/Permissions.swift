import ApplicationServices
import CoreGraphics
import AppKit

// The two TCC grants this app needs. Neither can be scripted away — a human must
// click "Allow" in System Settings once. After the first grant to an unsigned or
// ad-hoc build, relaunch so the process picks up the new trust status.
enum Permissions {
    // MARK: Accessibility (AX tree read + CGEvent input)

    static var accessibilityGranted: Bool { AXIsProcessTrusted() }

    /// Triggers the Accessibility prompt (adds the app to the list, unchecked).
    /// Uses the literal key string to sidestep the CFString/Unmanaged import
    /// differences of kAXTrustedCheckOptionPrompt across SDK versions.
    static func promptAccessibility() {
        let options = ["AXTrustedCheckOptionPrompt": true] as CFDictionary
        _ = AXIsProcessTrustedWithOptions(options)
    }

    // MARK: Screen Recording (ScreenCaptureKit)

    static var screenRecordingGranted: Bool { CGPreflightScreenCaptureAccess() }

    /// Triggers the Screen Recording prompt on first call.
    @discardableResult
    static func requestScreenRecording() -> Bool { CGRequestScreenCaptureAccess() }

    // MARK: Deep links to the relevant System Settings panes

    static func openAccessibilitySettings() { open("Privacy_Accessibility") }
    static func openScreenRecordingSettings() { open("Privacy_ScreenCapture") }

    private static func open(_ anchor: String) {
        if let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?\(anchor)") {
            NSWorkspace.shared.open(url)
        }
    }
}
