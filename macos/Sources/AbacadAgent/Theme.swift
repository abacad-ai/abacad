import SwiftUI

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/// Abacad design tokens. The menu-bar panel keeps native macOS materials for
/// its chrome; these tokens supply the shared semantic colors (status, brand)
/// and metrics so the panel reads as the same product as the dashboard.
enum Theme {
    static let canvas = Color(.sRGB, red: 0.0275, green: 0.0353, blue: 0.0510, opacity: 1.0000)
    static let sidebar = Color(.sRGB, red: 0.0431, green: 0.0549, blue: 0.0745, opacity: 1.0000)
    static let surface = Color(.sRGB, red: 0.0549, green: 0.0706, blue: 0.0941, opacity: 1.0000)
    static let surfaceRaised = Color(.sRGB, red: 0.0824, green: 0.1059, blue: 0.1412, opacity: 1.0000)
    static let surfaceHover = Color(.sRGB, red: 0.1098, green: 0.1412, blue: 0.1922, opacity: 1.0000)
    static let border = Color(.sRGB, red: 0.1333, green: 0.1686, blue: 0.2196, opacity: 1.0000)
    static let borderStrong = Color(.sRGB, red: 0.2000, green: 0.2471, blue: 0.3137, opacity: 1.0000)
    static let ink = Color(.sRGB, red: 0.9294, green: 0.9451, blue: 0.9686, opacity: 1.0000)
    static let inkMuted = Color(.sRGB, red: 0.5961, green: 0.6431, blue: 0.7098, opacity: 1.0000)
    static let inkSubtle = Color(.sRGB, red: 0.3922, green: 0.4314, blue: 0.4941, opacity: 1.0000)
    static let brand = Color(.sRGB, red: 0.9608, green: 0.7216, blue: 0.2392, opacity: 1.0000)
    static let brandStrong = Color(.sRGB, red: 0.8784, green: 0.6196, blue: 0.1176, opacity: 1.0000)
    static let brandSoft = Color(.sRGB, red: 0.1725, green: 0.1412, blue: 0.0784, opacity: 1.0000)
    static let onBrand = Color(.sRGB, red: 0.1020, green: 0.0706, blue: 0.0235, opacity: 1.0000)
    static let success = Color(.sRGB, red: 0.2902, green: 0.8706, blue: 0.5020, opacity: 1.0000)
    static let successStrong = Color(.sRGB, red: 0.1333, green: 0.7725, blue: 0.3686, opacity: 1.0000)
    static let successSoft = Color(.sRGB, red: 0.0627, green: 0.1529, blue: 0.1137, opacity: 1.0000)
    static let warning = Color(.sRGB, red: 0.9843, green: 0.5725, blue: 0.2353, opacity: 1.0000)
    static let warningStrong = Color(.sRGB, red: 0.9176, green: 0.3451, blue: 0.0471, opacity: 1.0000)
    static let warningSoft = Color(.sRGB, red: 0.1608, green: 0.1098, blue: 0.0784, opacity: 1.0000)
    static let danger = Color(.sRGB, red: 0.9725, green: 0.4431, blue: 0.4431, opacity: 1.0000)
    static let dangerStrong = Color(.sRGB, red: 0.8627, green: 0.1490, blue: 0.1490, opacity: 1.0000)
    static let dangerSoft = Color(.sRGB, red: 0.1686, green: 0.0980, blue: 0.1098, opacity: 1.0000)
    static let dangerHover = Color(.sRGB, red: 0.2275, green: 0.1216, blue: 0.1412, opacity: 1.0000)
    static let scrim = Color(.sRGB, red: 0.0000, green: 0.0000, blue: 0.0000, opacity: 0.6667)

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
