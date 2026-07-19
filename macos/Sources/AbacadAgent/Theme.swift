import SwiftUI

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/// Abacad design tokens. The menu-bar panel keeps native macOS materials for
/// its chrome; these tokens supply the shared semantic colors (status, brand)
/// and metrics so the panel reads as the same product as the dashboard.
enum Theme {
    static let canvas = Color(.sRGB, red: 0.0431, green: 0.0510, blue: 0.0627, opacity: 1.0000)
    static let sidebar = Color(.sRGB, red: 0.0627, green: 0.0745, blue: 0.0941, opacity: 1.0000)
    static let surface = Color(.sRGB, red: 0.0824, green: 0.0980, blue: 0.1216, opacity: 1.0000)
    static let surfaceRaised = Color(.sRGB, red: 0.1020, green: 0.1216, blue: 0.1529, opacity: 1.0000)
    static let surfaceHover = Color(.sRGB, red: 0.1255, green: 0.1490, blue: 0.1882, opacity: 1.0000)
    static let border = Color(.sRGB, red: 0.1569, green: 0.1843, blue: 0.2235, opacity: 1.0000)
    static let borderStrong = Color(.sRGB, red: 0.2275, green: 0.2667, blue: 0.3216, opacity: 1.0000)
    static let ink = Color(.sRGB, red: 0.9569, green: 0.9686, blue: 0.9843, opacity: 1.0000)
    static let inkMuted = Color(.sRGB, red: 0.6549, green: 0.6902, blue: 0.7412, opacity: 1.0000)
    static let inkSubtle = Color(.sRGB, red: 0.4471, green: 0.4902, blue: 0.5490, opacity: 1.0000)
    static let brand = Color(.sRGB, red: 0.4039, green: 0.9098, blue: 0.6471, opacity: 1.0000)
    static let brandStrong = Color(.sRGB, red: 0.1843, green: 0.7804, blue: 0.4941, opacity: 1.0000)
    static let brandSoft = Color(.sRGB, red: 0.0824, green: 0.2392, blue: 0.1765, opacity: 1.0000)
    static let onBrand = Color(.sRGB, red: 0.0275, green: 0.0745, blue: 0.0510, opacity: 1.0000)
    static let success = Color(.sRGB, red: 0.4039, green: 0.9098, blue: 0.6471, opacity: 1.0000)
    static let successStrong = Color(.sRGB, red: 0.1843, green: 0.7804, blue: 0.4941, opacity: 1.0000)
    static let successSoft = Color(.sRGB, red: 0.0784, green: 0.2078, blue: 0.1569, opacity: 1.0000)
    static let warning = Color(.sRGB, red: 0.9608, green: 0.7882, blue: 0.4196, opacity: 1.0000)
    static let warningStrong = Color(.sRGB, red: 0.7922, green: 0.5412, blue: 0.0157, opacity: 1.0000)
    static let warningSoft = Color(.sRGB, red: 0.2392, green: 0.1882, blue: 0.0902, opacity: 1.0000)
    static let danger = Color(.sRGB, red: 1.0000, green: 0.5412, blue: 0.5412, opacity: 1.0000)
    static let dangerStrong = Color(.sRGB, red: 0.8627, green: 0.1490, blue: 0.1490, opacity: 1.0000)
    static let dangerSoft = Color(.sRGB, red: 0.2588, green: 0.1216, blue: 0.1412, opacity: 1.0000)
    static let dangerHover = Color(.sRGB, red: 0.3333, green: 0.1490, blue: 0.1765, opacity: 1.0000)
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
