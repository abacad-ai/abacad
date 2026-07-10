// swift-tools-version:5.9
import PackageDescription

// Abacad macOS agent — a menu-bar app that dials the Abacad relay over a
// WebSocket and drives this Mac on command (AX tree, screen capture, CGEvent
// input), the desktop analogue of the Android AccessibilityService client.
//
// All dependencies are system frameworks (SwiftUI, AppKit, ScreenCaptureKit,
// ApplicationServices, CoreGraphics, Carbon, Network) — no external packages.
// `swift build` produces a bare binary; the Makefile wraps it into a signed
// .app bundle so TCC permissions (Accessibility, Screen Recording) attach to a
// stable identity.
let package = Package(
    name: "AbacadAgent",
    platforms: [.macOS(.v14)], // SCScreenshotManager.captureImage needs macOS 14
    targets: [
        .executableTarget(
            name: "AbacadAgent",
            path: "Sources/AbacadAgent"
        )
    ]
)
