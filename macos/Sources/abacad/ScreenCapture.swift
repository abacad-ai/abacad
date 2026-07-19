import ScreenCaptureKit
import AppKit
import CoreGraphics

// Captures the main display with ScreenCaptureKit (the current API; the old
// CGDisplayCreateImage is deprecated) and encodes to JPEG. Requires the Screen
// Recording permission.
//
// The capture is configured at the display's POINT size, not its native pixel
// size, so the returned image dimensions match the coordinate space used by the
// accessibility tree bounds and CGEvent input. That preserves the Android
// invariant: 1 screenshot pixel == 1 click unit. Field name stays `png_base64`
// for wire compatibility even though the bytes are JPEG.
enum ScreenCapture {
    struct Shot { let w: Int; let h: Int; let base64: String }

    static func capture(jpegQuality: Double = 0.7) async throws -> Shot {
        let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: false)
        guard let display = content.displays.first else {
            throw CmdError.message("no display available")
        }
        let filter = SCContentFilter(display: display, excludingWindows: [])
        let config = SCStreamConfiguration()
        // Capture at POINT resolution (from the main screen's frame), not native
        // Retina pixels, so image dimensions equal the AX/CGEvent coordinate space
        // — 1 screenshot pixel == 1 click unit, matching the Android invariant.
        let pointSize = NSScreen.main?.frame.size ?? CGSize(width: display.width, height: display.height)
        config.width = Int(pointSize.width)
        config.height = Int(pointSize.height)
        config.showsCursor = true

        let cgImage = try await SCScreenshotManager.captureImage(contentFilter: filter, configuration: config)
        guard let data = jpeg(cgImage, quality: jpegQuality) else {
            throw CmdError.message("jpeg encode failed")
        }
        return Shot(w: cgImage.width, h: cgImage.height, base64: data.base64EncodedString())
    }

    private static func jpeg(_ image: CGImage, quality: Double) -> Data? {
        let rep = NSBitmapImageRep(cgImage: image)
        return rep.representation(using: .jpeg, properties: [.compressionFactor: quality])
    }
}
