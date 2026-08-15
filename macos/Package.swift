// swift-tools-version:5.9
import PackageDescription

// abacad macOS agent — a menu-bar app that dials the abacad relay over a
// WebSocket and drives this Mac on command (AX tree, screen capture, CGEvent
// input), the desktop analogue of the Android AccessibilityService client.
//
// All dependencies are system frameworks (SwiftUI, AppKit, ScreenCaptureKit,
// ApplicationServices, CoreGraphics, Carbon, Network, ServiceManagement) — no
// external packages.
// `swift build` produces bare binaries; the Makefile wraps `abacad` into a signed
// .app bundle so TCC permissions (Accessibility, Screen Recording) attach to a
// stable identity.
//
// Three targets, because the machine is driven by the app and configured by
// either:
//
//   AbacadKit  — the UI-free half: pairing, enrollment, the capability ceiling,
//                Keychain-backed prefs, JSON helpers. Foundation only.
//   abacad     — the menu-bar app. Does the actual seeing and controlling; that
//                work stays here because TCC keys Screen Recording and
//                Accessibility to the .app bundle's identity, so a bare binary
//                cannot hold those grants in any durable way.
//   abacad-cli — the terminal front-end: `connect`, `capabilities`, `status`.
//                Shares the Keychain store (service ai.abacad.agent, no access
//                group) with the app, so configuring from either side is the
//                same configuration.
let package = Package(
    name: "abacad",
    platforms: [.macOS(.v14)], // SCScreenshotManager.captureImage needs macOS 14
    targets: [
        .target(
            name: "AbacadKit",
            path: "Sources/AbacadKit"
        ),
        .executableTarget(
            name: "abacad",
            dependencies: ["AbacadKit"],
            path: "Sources/abacad"
        ),
        .executableTarget(
            name: "abacad-cli",
            dependencies: ["AbacadKit"],
            path: "Sources/abacad-cli"
        ),
    ]
)
