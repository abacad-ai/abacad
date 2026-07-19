import AppKit
import SwiftUI

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/// Abacad design tokens. The menu-bar panel keeps native macOS materials for
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

    static let canvas = dynamic(dark: (0.0275, 0.0353, 0.0510, 1.0000), light: (0.9647, 0.9686, 0.9765, 1.0000))
    static let sidebar = dynamic(dark: (0.0431, 0.0549, 0.0745, 1.0000), light: (0.9333, 0.9412, 0.9569, 1.0000))
    static let surface = dynamic(dark: (0.0549, 0.0706, 0.0941, 1.0000), light: (1.0000, 1.0000, 1.0000, 1.0000))
    static let surfaceRaised = dynamic(dark: (0.0824, 0.1059, 0.1412, 1.0000), light: (0.9529, 0.9608, 0.9725, 1.0000))
    static let surfaceHover = dynamic(dark: (0.1098, 0.1412, 0.1922, 1.0000), light: (0.9137, 0.9294, 0.9490, 1.0000))
    static let border = dynamic(dark: (0.1333, 0.1686, 0.2196, 1.0000), light: (0.8627, 0.8824, 0.9137, 1.0000))
    static let borderStrong = dynamic(dark: (0.2000, 0.2471, 0.3137, 1.0000), light: (0.7216, 0.7608, 0.8118, 1.0000))
    static let ink = dynamic(dark: (0.9294, 0.9451, 0.9686, 1.0000), light: (0.0902, 0.1137, 0.1490, 1.0000))
    static let inkMuted = dynamic(dark: (0.5961, 0.6431, 0.7098, 1.0000), light: (0.3529, 0.3961, 0.4667, 1.0000))
    static let inkSubtle = dynamic(dark: (0.3922, 0.4314, 0.4941, 1.0000), light: (0.5294, 0.5725, 0.6353, 1.0000))
    static let brand = dynamic(dark: (0.9608, 0.7216, 0.2392, 1.0000), light: (0.6314, 0.3843, 0.0275, 1.0000))
    static let brandStrong = dynamic(dark: (0.8784, 0.6196, 0.1176, 1.0000), light: (0.4863, 0.3020, 0.0196, 1.0000))
    static let brandSoft = dynamic(dark: (0.1725, 0.1412, 0.0784, 1.0000), light: (0.9725, 0.9333, 0.8275, 1.0000))
    static let onBrand = dynamic(dark: (0.1020, 0.0706, 0.0235, 1.0000), light: (1.0000, 1.0000, 1.0000, 1.0000))
    static let success = dynamic(dark: (0.2902, 0.8706, 0.5020, 1.0000), light: (0.0824, 0.5020, 0.2392, 1.0000))
    static let successStrong = dynamic(dark: (0.1333, 0.7725, 0.3686, 1.0000), light: (0.0667, 0.3961, 0.1922, 1.0000))
    static let successSoft = dynamic(dark: (0.0627, 0.1529, 0.1137, 1.0000), light: (0.8745, 0.9529, 0.9020, 1.0000))
    static let warning = dynamic(dark: (0.9843, 0.5725, 0.2353, 1.0000), light: (0.7059, 0.3255, 0.0353, 1.0000))
    static let warningStrong = dynamic(dark: (0.9176, 0.3451, 0.0471, 1.0000), light: (0.5608, 0.2510, 0.0314, 1.0000))
    static let warningSoft = dynamic(dark: (0.1608, 0.1098, 0.0784, 1.0000), light: (0.9843, 0.9333, 0.8549, 1.0000))
    static let danger = dynamic(dark: (0.9725, 0.4431, 0.4431, 1.0000), light: (0.8627, 0.1490, 0.1490, 1.0000))
    static let dangerStrong = dynamic(dark: (0.8627, 0.1490, 0.1490, 1.0000), light: (0.7255, 0.1098, 0.1098, 1.0000))
    static let dangerSoft = dynamic(dark: (0.1686, 0.0980, 0.1098, 1.0000), light: (0.9882, 0.9098, 0.9098, 1.0000))
    static let dangerHover = dynamic(dark: (0.2275, 0.1216, 0.1412, 1.0000), light: (0.9725, 0.8549, 0.8549, 1.0000))
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
