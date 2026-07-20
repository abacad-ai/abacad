import AppKit

// abacad relay mark, drawn with AppKit so the menu-bar item is our own graph
// rather than an SF Symbol. Four connected dots (one hub -> three devices) that
// read as an "A". The tray image is a *template* (monochrome) so macOS tints it
// automatically for light/dark menu bars; connection state is shown structurally
// — a filled hub when connected, a hollow ring when offline — so it stays mono.
//
// Keep the geometry in sync with assets/icon.svg. (This is a compact, slightly
// bolder layout tuned to read at ~18pt; the proportions match the wide-balanced
// mark, not a pixel-for-pixel copy of the 512px art.)
enum RelayMark {
    // Unit-square coordinates, y-down (we draw into a flipped context).
    private static let apex  = CGPoint(x: 0.50, y: 0.20)
    private static let footL = CGPoint(x: 0.20, y: 0.80)
    private static let footR = CGPoint(x: 0.80, y: 0.80)
    private static let hub   = CGPoint(x: 0.50, y: 0.55)
    private static let nodeR: CGFloat = 0.085
    private static let hubR:  CGFloat = 0.100
    private static let edgeW: CGFloat = 0.045

    /// Monochrome menu-bar icon. `connected` fills the hub; otherwise it's a ring.
    static func trayImage(connected: Bool, points: CGFloat = 18) -> NSImage {
        let image = NSImage(size: NSSize(width: points, height: points), flipped: true) { rect in
            let s = rect.width
            func p(_ pt: CGPoint) -> NSPoint { NSPoint(x: pt.x * s, y: pt.y * s) }

            // Edges: hub -> each device node, half strength so the dots dominate.
            let edges = NSBezierPath()
            edges.lineWidth = edgeW * s
            edges.lineCapStyle = .round
            for foot in [apex, footL, footR] {
                edges.move(to: p(hub))
                edges.line(to: p(foot))
            }
            NSColor.black.withAlphaComponent(0.5).setStroke()
            edges.stroke()

            // Device nodes (silver in the art; solid tint here).
            NSColor.black.setFill()
            for foot in [apex, footL, footR] {
                circle(center: p(foot), radius: nodeR * s).fill()
            }

            // Hub = "alive". Filled when connected, hollow ring when offline.
            let hubPath = circle(center: p(hub), radius: hubR * s)
            if connected {
                hubPath.fill()
            } else {
                hubPath.lineWidth = edgeW * s * 1.1
                hubPath.stroke()
            }
            return true
        }
        image.isTemplate = true // let the menu bar tint it (black on light, white on dark)
        return image
    }

    private static func circle(center: NSPoint, radius: CGFloat) -> NSBezierPath {
        NSBezierPath(ovalIn: NSRect(x: center.x - radius, y: center.y - radius,
                                    width: radius * 2, height: radius * 2))
    }
}
