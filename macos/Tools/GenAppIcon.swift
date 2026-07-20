// App-icon generator. Draws the abacad relay mark (charcoal tile + silver graph +
// green "alive" hub) at every iconset size using AppKit, so building the .icns
// needs no external rasterizer — only the Swift toolchain the app already requires.
//
//   swift Tools/GenAppIcon.swift <out.iconset>   # then: iconutil -c icns <out.iconset>
//
// `make icon` wraps both steps and writes macos/AppIcon.icns (committed, so `make
// app` bundles it without regenerating). Geometry mirrors assets/icon.svg — keep
// them in sync if the mark changes.
import AppKit
import Foundation

let outDir = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "AppIcon.iconset"
try? FileManager.default.createDirectory(atPath: outDir, withIntermediateDirectories: true)

let charcoal = NSColor(srgbRed: 0x0b/255.0, green: 0x0b/255.0, blue: 0x0d/255.0, alpha: 1)
let silver   = NSColor(srgbRed: 0xd8/255.0, green: 0xda/255.0, blue: 0xde/255.0, alpha: 1)
let green    = NSColor(srgbRed: 0x30/255.0, green: 0xd1/255.0, blue: 0x58/255.0, alpha: 1)

// 512-space geometry (matches assets/icon.svg; y includes the +10 centering shift).
let apex  = CGPoint(x: 256, y: 126), footL = CGPoint(x: 108, y: 370)
let footR = CGPoint(x: 404, y: 370), hub   = CGPoint(x: 256, y: 272)

func circle(_ c: NSPoint, _ r: CGFloat) -> NSBezierPath {
    NSBezierPath(ovalIn: NSRect(x: c.x - r, y: c.y - r, width: 2 * r, height: 2 * r))
}

func drawIcon(_ px: Int) -> NSBitmapImageRep {
    let rep = NSBitmapImageRep(bitmapDataPlanes: nil, pixelsWide: px, pixelsHigh: px,
        bitsPerSample: 8, samplesPerPixel: 4, hasAlpha: true, isPlanar: false,
        colorSpaceName: .deviceRGB, bytesPerRow: 0, bitsPerPixel: 0)!
    NSGraphicsContext.saveGraphicsState()
    NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
    let s = CGFloat(px) / 512.0
    func p(_ pt: CGPoint) -> NSPoint { NSPoint(x: pt.x * s, y: (512 - pt.y) * s) } // flip to y-up

    charcoal.setFill()
    NSBezierPath(roundedRect: NSRect(x: 0, y: 0, width: 512 * s, height: 512 * s),
                 xRadius: 114 * s, yRadius: 114 * s).fill()

    let edges = NSBezierPath(); edges.lineWidth = 10 * s; edges.lineCapStyle = .round
    for f in [apex, footL, footR] { edges.move(to: p(hub)); edges.line(to: p(f)) }
    silver.withAlphaComponent(0.5).setStroke(); edges.stroke()

    silver.setFill()
    for f in [apex, footL, footR] { circle(p(f), 28 * s).fill() }

    green.withAlphaComponent(0.16).setFill(); circle(p(hub), 56 * s).fill()
    green.setFill(); circle(p(hub), 34 * s).fill()

    NSGraphicsContext.restoreGraphicsState()
    return rep
}

// (name, pixel size) — the exact set `iconutil` expects for a .icns.
let specs: [(String, Int)] = [
    ("icon_16x16", 16), ("icon_16x16@2x", 32),
    ("icon_32x32", 32), ("icon_32x32@2x", 64),
    ("icon_128x128", 128), ("icon_128x128@2x", 256),
    ("icon_256x256", 256), ("icon_256x256@2x", 512),
    ("icon_512x512", 512), ("icon_512x512@2x", 1024),
]
for (name, px) in specs {
    let data = drawIcon(px).representation(using: .png, properties: [:])!
    try! data.write(to: URL(fileURLWithPath: "\(outDir)/\(name).png"))
}
print("wrote \(specs.count) PNGs to \(outDir)")
