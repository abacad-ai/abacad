package ai.abacad.android

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/** Every abacad token as a Compose [Color]. Status colors (success/warning/
 * danger) are used by name for the connection-state UI; the neutral chrome is
 * also mapped onto Material 3 via [abacadColorScheme]. */
data class AbacadColors(
    val canvas: Color,
    val sidebar: Color,
    val surface: Color,
    val surfaceRaised: Color,
    val surfaceHover: Color,
    val border: Color,
    val borderStrong: Color,
    val ink: Color,
    val inkMuted: Color,
    val inkSubtle: Color,
    val brand: Color,
    val brandStrong: Color,
    val brandSoft: Color,
    val onBrand: Color,
    val success: Color,
    val successStrong: Color,
    val successSoft: Color,
    val warning: Color,
    val warningStrong: Color,
    val warningSoft: Color,
    val danger: Color,
    val dangerStrong: Color,
    val dangerSoft: Color,
    val dangerHover: Color,
    val scrim: Color,
    val shadow: Color,
    val shadowStrong: Color,
)

val AbacadDark = AbacadColors(
    canvas = Color(0xFF0B0B0D),
    sidebar = Color(0xFF0E0E11),
    surface = Color(0xFF161618),
    surfaceRaised = Color(0xFF1C1C1F),
    surfaceHover = Color(0xFF242427),
    border = Color(0xFF2A2A2E),
    borderStrong = Color(0xFF3C3C40),
    ink = Color(0xFFF2F2F4),
    inkMuted = Color(0xFF9A9AA0),
    inkSubtle = Color(0xFF66666C),
    brand = Color(0xFFD8DADE),
    brandStrong = Color(0xFFFFFFFF),
    brandSoft = Color(0xFF202024),
    onBrand = Color(0xFF0B0B0D),
    success = Color(0xFF30D158),
    successStrong = Color(0xFF28B84C),
    successSoft = Color(0xFF0F2A19),
    warning = Color(0xFFFF9F0A),
    warningStrong = Color(0xFFD98207),
    warningSoft = Color(0xFF2E2109),
    danger = Color(0xFFFF453A),
    dangerStrong = Color(0xFFE0342B),
    dangerSoft = Color(0xFF2F1210),
    dangerHover = Color(0xFF3D1A17),
    scrim = Color(0xAA000000),
    shadow = Color(0x38000000),
    shadowStrong = Color(0x73000000),
)

val AbacadLight = AbacadColors(
    canvas = Color(0xFFF5F5F7),
    sidebar = Color(0xFFECECEF),
    surface = Color(0xFFFFFFFF),
    surfaceRaised = Color(0xFFFBFBFD),
    surfaceHover = Color(0xFFECECEF),
    border = Color(0xFFD2D2D7),
    borderStrong = Color(0xFFB7B7BE),
    ink = Color(0xFF1D1D1F),
    inkMuted = Color(0xFF6E6E73),
    inkSubtle = Color(0xFF86868B),
    brand = Color(0xFF1D1D1F),
    brandStrong = Color(0xFF000000),
    brandSoft = Color(0xFFE8E8EA),
    onBrand = Color(0xFFFFFFFF),
    success = Color(0xFF248A3D),
    successStrong = Color(0xFF1C6E30),
    successSoft = Color(0xFFE2F3E6),
    warning = Color(0xFFB25000),
    warningStrong = Color(0xFF8F4008),
    warningSoft = Color(0xFFFBEEDE),
    danger = Color(0xFFD70015),
    dangerStrong = Color(0xFFB00010),
    dangerSoft = Color(0xFFFBE5E6),
    dangerHover = Color(0xFFF7D3D6),
    scrim = Color(0xAA000000),
    shadow = Color(0x141B2634),
    shadowStrong = Color(0x211B2634),
)

fun abacadColors(dark: Boolean): AbacadColors = if (dark) AbacadDark else AbacadLight

/** Map the neutral chrome + error tokens onto a Material 3 [ColorScheme]. */
fun abacadColorScheme(dark: Boolean): ColorScheme {
    val c = abacadColors(dark)
    return if (dark) darkColorScheme(
        background = c.canvas,
        onBackground = c.ink,
        surface = c.surface,
        onSurface = c.ink,
        surfaceVariant = c.surfaceRaised,
        onSurfaceVariant = c.inkMuted,
        primary = c.brand,
        onPrimary = c.onBrand,
        outline = c.border,
        outlineVariant = c.borderStrong,
        error = c.danger,
        errorContainer = c.dangerSoft,
        scrim = c.scrim,
    ) else lightColorScheme(
        background = c.canvas,
        onBackground = c.ink,
        surface = c.surface,
        onSurface = c.ink,
        surfaceVariant = c.surfaceRaised,
        onSurfaceVariant = c.inkMuted,
        primary = c.brand,
        onPrimary = c.onBrand,
        outline = c.border,
        outlineVariant = c.borderStrong,
        error = c.danger,
        errorContainer = c.dangerSoft,
        scrim = c.scrim,
    )
}

/** Spacing (dp), corner radii (dp) and type sizes (sp). */
object AbacadDim {
    val spaceXs = 4.dp
    val spaceSm = 8.dp
    val spaceMd = 12.dp
    val spaceLg = 16.dp
    val spaceXl = 24.dp
    val radiusSm = 6.dp
    val radiusMd = 10.dp
    val radiusPill = 999.dp
    val textXs = 12.sp
    val textSm = 13.sp
    val textMd = 15.sp
    val textLg = 17.sp
}
