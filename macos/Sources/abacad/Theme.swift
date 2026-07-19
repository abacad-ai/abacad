import AppKit
import SwiftUI

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/// abacad design tokens. The menu-bar panel keeps native macOS materials for
/// its chrome; these tokens supply the shared semantic colors (status, brand)
/// and metrics so the panel reads as the same product as the dashboard. Each
/// color is appearance-dynamic — it resolves to the dark or light variant as
/// the system (or panel) appearance changes, so auto dark/light needs no code.
enum Theme {
    private typealias RGBA = (r: CGFloat, g: CGFloat, b: CGFloat, a: CGFloat)

    private static func dynamic(dark: RGBA, light: RGBA) -> Color {
        Color(nsColor: NSColor(name: nil) { appearance in
            let c = appearance.bestMatch(from: [.darkAqua, .aqua]) == .aqua ? light : dark
            return NSColor(srgbRed: c.r, green: c.g, blue: c.b, alpha: c.a)
        })
    }

    static let canvas = dynamic(dark: (0.0431, 0.0431, 0.0510, 1.0000), light: (0.9608, 0.9608, 0.9686, 1.0000))
    static let sidebar = dynamic(dark: (0.0549, 0.0549, 0.0667, 1.0000), light: (0.9255, 0.9255, 0.9373, 1.0000))
    static let surface = dynamic(dark: (0.0863, 0.0863, 0.0941, 1.0000), light: (1.0000, 1.0000, 1.0000, 1.0000))
    static let surfaceRaised = dynamic(dark: (0.1098, 0.1098, 0.1216, 1.0000), light: (0.9843, 0.9843, 0.9922, 1.0000))
    static let surfaceHover = dynamic(dark: (0.1412, 0.1412, 0.1529, 1.0000), light: (0.9255, 0.9255, 0.9373, 1.0000))
    static let border = dynamic(dark: (0.1647, 0.1647, 0.1804, 1.0000), light: (0.8235, 0.8235, 0.8431, 1.0000))
    static let borderStrong = dynamic(dark: (0.2353, 0.2353, 0.2510, 1.0000), light: (0.7176, 0.7176, 0.7451, 1.0000))
    static let ink = dynamic(dark: (0.9490, 0.9490, 0.9569, 1.0000), light: (0.1137, 0.1137, 0.1216, 1.0000))
    static let inkMuted = dynamic(dark: (0.6039, 0.6039, 0.6275, 1.0000), light: (0.4314, 0.4314, 0.4510, 1.0000))
    static let inkSubtle = dynamic(dark: (0.4000, 0.4000, 0.4235, 1.0000), light: (0.5255, 0.5255, 0.5451, 1.0000))
    static let brand = dynamic(dark: (0.8471, 0.8549, 0.8706, 1.0000), light: (0.1137, 0.1137, 0.1216, 1.0000))
    static let brandStrong = dynamic(dark: (1.0000, 1.0000, 1.0000, 1.0000), light: (0.0000, 0.0000, 0.0000, 1.0000))
    static let brandSoft = dynamic(dark: (0.1255, 0.1255, 0.1412, 1.0000), light: (0.9098, 0.9098, 0.9176, 1.0000))
    static let onBrand = dynamic(dark: (0.0431, 0.0431, 0.0510, 1.0000), light: (1.0000, 1.0000, 1.0000, 1.0000))
    static let success = dynamic(dark: (0.1882, 0.8196, 0.3451, 1.0000), light: (0.1412, 0.5412, 0.2392, 1.0000))
    static let successStrong = dynamic(dark: (0.1569, 0.7216, 0.2980, 1.0000), light: (0.1098, 0.4314, 0.1882, 1.0000))
    static let successSoft = dynamic(dark: (0.0588, 0.1647, 0.0980, 1.0000), light: (0.8863, 0.9529, 0.9020, 1.0000))
    static let warning = dynamic(dark: (1.0000, 0.6235, 0.0392, 1.0000), light: (0.6980, 0.3137, 0.0000, 1.0000))
    static let warningStrong = dynamic(dark: (0.8510, 0.5098, 0.0275, 1.0000), light: (0.5608, 0.2510, 0.0314, 1.0000))
    static let warningSoft = dynamic(dark: (0.1804, 0.1294, 0.0353, 1.0000), light: (0.9843, 0.9333, 0.8706, 1.0000))
    static let danger = dynamic(dark: (1.0000, 0.2706, 0.2275, 1.0000), light: (0.8431, 0.0000, 0.0824, 1.0000))
    static let dangerStrong = dynamic(dark: (0.8784, 0.2039, 0.1686, 1.0000), light: (0.6902, 0.0000, 0.0627, 1.0000))
    static let dangerSoft = dynamic(dark: (0.1843, 0.0706, 0.0627, 1.0000), light: (0.9843, 0.8980, 0.9020, 1.0000))
    static let dangerHover = dynamic(dark: (0.2392, 0.1020, 0.0902, 1.0000), light: (0.9686, 0.8275, 0.8392, 1.0000))
    static let scrim = dynamic(dark: (0.0000, 0.0000, 0.0000, 0.6667), light: (0.0000, 0.0000, 0.0000, 0.6667))
    static let shadow = dynamic(dark: (0.0000, 0.0000, 0.0000, 0.2196), light: (0.1059, 0.1490, 0.2039, 0.0784))
    static let shadowStrong = dynamic(dark: (0.0000, 0.0000, 0.0000, 0.4510), light: (0.1059, 0.1490, 0.2039, 0.1294))

    static let spaceXs: CGFloat = 4
    static let spaceSm: CGFloat = 8
    static let spaceMd: CGFloat = 12
    static let spaceLg: CGFloat = 16
    static let spaceXl: CGFloat = 24
    static let radiusSm: CGFloat = 6
    static let radiusMd: CGFloat = 10
    static let radiusPill: CGFloat = 999
    static let textXs: CGFloat = 12
    static let textSm: CGFloat = 13
    static let textMd: CGFloat = 15
    static let textLg: CGFloat = 17
}
