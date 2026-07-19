package ai.abacad.android

import android.content.res.Configuration
import android.content.res.Resources

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

/**
 * abacad design tokens. Colors are ARGB ints (pass straight to setTextColor /
 * setBackgroundColor) and come as a dark and a light [Palette] — call
 * [Theme.of] to get the one matching the current system appearance (auto
 * dark/light is the product default on every surface). SPACE_* are dp,
 * TEXT_* are sp — multiply dp values by displayMetrics.density before use.
 */
object Theme {
    class Palette internal constructor(
        val CANVAS: Int,
        val SIDEBAR: Int,
        val SURFACE: Int,
        val SURFACE_RAISED: Int,
        val SURFACE_HOVER: Int,
        val BORDER: Int,
        val BORDER_STRONG: Int,
        val INK: Int,
        val INK_MUTED: Int,
        val INK_SUBTLE: Int,
        val BRAND: Int,
        val BRAND_STRONG: Int,
        val BRAND_SOFT: Int,
        val ON_BRAND: Int,
        val SUCCESS: Int,
        val SUCCESS_STRONG: Int,
        val SUCCESS_SOFT: Int,
        val WARNING: Int,
        val WARNING_STRONG: Int,
        val WARNING_SOFT: Int,
        val DANGER: Int,
        val DANGER_STRONG: Int,
        val DANGER_SOFT: Int,
        val DANGER_HOVER: Int,
        val SCRIM: Int,
        val SHADOW: Int,
        val SHADOW_STRONG: Int,
    )

    val DARK = Palette(
        CANVAS = 0xFF0B0B0D.toInt(),
        SIDEBAR = 0xFF0E0E11.toInt(),
        SURFACE = 0xFF161618.toInt(),
        SURFACE_RAISED = 0xFF1C1C1F.toInt(),
        SURFACE_HOVER = 0xFF242427.toInt(),
        BORDER = 0xFF2A2A2E.toInt(),
        BORDER_STRONG = 0xFF3C3C40.toInt(),
        INK = 0xFFF2F2F4.toInt(),
        INK_MUTED = 0xFF9A9AA0.toInt(),
        INK_SUBTLE = 0xFF66666C.toInt(),
        BRAND = 0xFFD8DADE.toInt(),
        BRAND_STRONG = 0xFFFFFFFF.toInt(),
        BRAND_SOFT = 0xFF202024.toInt(),
        ON_BRAND = 0xFF0B0B0D.toInt(),
        SUCCESS = 0xFF30D158.toInt(),
        SUCCESS_STRONG = 0xFF28B84C.toInt(),
        SUCCESS_SOFT = 0xFF0F2A19.toInt(),
        WARNING = 0xFFFF9F0A.toInt(),
        WARNING_STRONG = 0xFFD98207.toInt(),
        WARNING_SOFT = 0xFF2E2109.toInt(),
        DANGER = 0xFFFF453A.toInt(),
        DANGER_STRONG = 0xFFE0342B.toInt(),
        DANGER_SOFT = 0xFF2F1210.toInt(),
        DANGER_HOVER = 0xFF3D1A17.toInt(),
        SCRIM = 0xAA000000.toInt(),
        SHADOW = 0x38000000.toInt(),
        SHADOW_STRONG = 0x73000000.toInt(),
    )

    val LIGHT = Palette(
        CANVAS = 0xFFF5F5F7.toInt(),
        SIDEBAR = 0xFFECECEF.toInt(),
        SURFACE = 0xFFFFFFFF.toInt(),
        SURFACE_RAISED = 0xFFFBFBFD.toInt(),
        SURFACE_HOVER = 0xFFECECEF.toInt(),
        BORDER = 0xFFD2D2D7.toInt(),
        BORDER_STRONG = 0xFFB7B7BE.toInt(),
        INK = 0xFF1D1D1F.toInt(),
        INK_MUTED = 0xFF6E6E73.toInt(),
        INK_SUBTLE = 0xFF86868B.toInt(),
        BRAND = 0xFF1D1D1F.toInt(),
        BRAND_STRONG = 0xFF000000.toInt(),
        BRAND_SOFT = 0xFFE8E8EA.toInt(),
        ON_BRAND = 0xFFFFFFFF.toInt(),
        SUCCESS = 0xFF248A3D.toInt(),
        SUCCESS_STRONG = 0xFF1C6E30.toInt(),
        SUCCESS_SOFT = 0xFFE2F3E6.toInt(),
        WARNING = 0xFFB25000.toInt(),
        WARNING_STRONG = 0xFF8F4008.toInt(),
        WARNING_SOFT = 0xFFFBEEDE.toInt(),
        DANGER = 0xFFD70015.toInt(),
        DANGER_STRONG = 0xFFB00010.toInt(),
        DANGER_SOFT = 0xFFFBE5E6.toInt(),
        DANGER_HOVER = 0xFFF7D3D6.toInt(),
        SCRIM = 0xAA000000.toInt(),
        SHADOW = 0x141B2634.toInt(),
        SHADOW_STRONG = 0x211B2634.toInt(),
    )

    /** The palette for the current system appearance (uiMode night flag). */
    fun of(resources: Resources): Palette {
        val night = resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK
        return if (night == Configuration.UI_MODE_NIGHT_NO) LIGHT else DARK
    }

    const val SPACE_XS = 4 // dp
    const val SPACE_SM = 8 // dp
    const val SPACE_MD = 12 // dp
    const val SPACE_LG = 16 // dp
    const val SPACE_XL = 24 // dp
    const val RADIUS_SM = 6 // dp
    const val RADIUS_MD = 10 // dp
    const val RADIUS_PILL = 999 // dp
    const val TEXT_XS = 12f // sp
    const val TEXT_SM = 13f // sp
    const val TEXT_MD = 15f // sp
    const val TEXT_LG = 17f // sp
}
