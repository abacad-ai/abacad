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
        CANVAS = 0xFF07090D.toInt(),
        SIDEBAR = 0xFF0B0E13.toInt(),
        SURFACE = 0xFF0E1218.toInt(),
        SURFACE_RAISED = 0xFF151B24.toInt(),
        SURFACE_HOVER = 0xFF1C2431.toInt(),
        BORDER = 0xFF222B38.toInt(),
        BORDER_STRONG = 0xFF333F50.toInt(),
        INK = 0xFFEDF1F7.toInt(),
        INK_MUTED = 0xFF98A4B5.toInt(),
        INK_SUBTLE = 0xFF646E7E.toInt(),
        BRAND = 0xFFF5B83D.toInt(),
        BRAND_STRONG = 0xFFE09E1E.toInt(),
        BRAND_SOFT = 0xFF2C2414.toInt(),
        ON_BRAND = 0xFF1A1206.toInt(),
        SUCCESS = 0xFF4ADE80.toInt(),
        SUCCESS_STRONG = 0xFF22C55E.toInt(),
        SUCCESS_SOFT = 0xFF10271D.toInt(),
        WARNING = 0xFFFB923C.toInt(),
        WARNING_STRONG = 0xFFEA580C.toInt(),
        WARNING_SOFT = 0xFF291C14.toInt(),
        DANGER = 0xFFF87171.toInt(),
        DANGER_STRONG = 0xFFDC2626.toInt(),
        DANGER_SOFT = 0xFF2B191C.toInt(),
        DANGER_HOVER = 0xFF3A1F24.toInt(),
        SCRIM = 0xAA000000.toInt(),
        SHADOW = 0x38000000.toInt(),
        SHADOW_STRONG = 0x73000000.toInt(),
    )

    val LIGHT = Palette(
        CANVAS = 0xFFF6F7F9.toInt(),
        SIDEBAR = 0xFFEEF0F4.toInt(),
        SURFACE = 0xFFFFFFFF.toInt(),
        SURFACE_RAISED = 0xFFF3F5F8.toInt(),
        SURFACE_HOVER = 0xFFE9EDF2.toInt(),
        BORDER = 0xFFDCE1E9.toInt(),
        BORDER_STRONG = 0xFFB8C2CF.toInt(),
        INK = 0xFF171D26.toInt(),
        INK_MUTED = 0xFF5A6577.toInt(),
        INK_SUBTLE = 0xFF8792A2.toInt(),
        BRAND = 0xFFA16207.toInt(),
        BRAND_STRONG = 0xFF7C4D05.toInt(),
        BRAND_SOFT = 0xFFF8EED3.toInt(),
        ON_BRAND = 0xFFFFFFFF.toInt(),
        SUCCESS = 0xFF15803D.toInt(),
        SUCCESS_STRONG = 0xFF116531.toInt(),
        SUCCESS_SOFT = 0xFFDFF3E6.toInt(),
        WARNING = 0xFFB45309.toInt(),
        WARNING_STRONG = 0xFF8F4008.toInt(),
        WARNING_SOFT = 0xFFFBEEDA.toInt(),
        DANGER = 0xFFDC2626.toInt(),
        DANGER_STRONG = 0xFFB91C1C.toInt(),
        DANGER_SOFT = 0xFFFCE8E8.toInt(),
        DANGER_HOVER = 0xFFF8DADA.toInt(),
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
