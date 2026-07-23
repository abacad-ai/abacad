package gui

// GENERATED from design/tokens.json — do not edit. Run: node design/generate.mjs

// Palette is one appearance's abacad tokens as CSS-ready hex strings
// ("#rrggbb" or "#rrggbbaa"). Feed them to a GTK CssProvider or gdk.RGBA.Parse.
type Palette struct {
	Canvas        string
	Sidebar       string
	Surface       string
	SurfaceRaised string
	SurfaceHover  string
	Border        string
	BorderStrong  string
	Ink           string
	InkMuted      string
	InkSubtle     string
	Brand         string
	BrandStrong   string
	BrandSoft     string
	OnBrand       string
	Success       string
	SuccessStrong string
	SuccessSoft   string
	Warning       string
	WarningStrong string
	WarningSoft   string
	Danger        string
	DangerStrong  string
	DangerSoft    string
	DangerHover   string
	Scrim         string
	Shadow        string
	ShadowStrong  string
}

var Dark = Palette{
	Canvas:        "#0b0b0d",
	Sidebar:       "#0e0e11",
	Surface:       "#161618",
	SurfaceRaised: "#1c1c1f",
	SurfaceHover:  "#242427",
	Border:        "#2a2a2e",
	BorderStrong:  "#3c3c40",
	Ink:           "#f2f2f4",
	InkMuted:      "#9a9aa0",
	InkSubtle:     "#66666c",
	Brand:         "#d8dade",
	BrandStrong:   "#ffffff",
	BrandSoft:     "#202024",
	OnBrand:       "#0b0b0d",
	Success:       "#30d158",
	SuccessStrong: "#28b84c",
	SuccessSoft:   "#0f2a19",
	Warning:       "#ff9f0a",
	WarningStrong: "#d98207",
	WarningSoft:   "#2e2109",
	Danger:        "#ff453a",
	DangerStrong:  "#e0342b",
	DangerSoft:    "#2f1210",
	DangerHover:   "#3d1a17",
	Scrim:         "#000000aa",
	Shadow:        "#00000038",
	ShadowStrong:  "#00000073",
}

var Light = Palette{
	Canvas:        "#f5f5f7",
	Sidebar:       "#ececef",
	Surface:       "#ffffff",
	SurfaceRaised: "#fbfbfd",
	SurfaceHover:  "#ececef",
	Border:        "#d2d2d7",
	BorderStrong:  "#b7b7be",
	Ink:           "#1d1d1f",
	InkMuted:      "#6e6e73",
	InkSubtle:     "#86868b",
	Brand:         "#1d1d1f",
	BrandStrong:   "#000000",
	BrandSoft:     "#e8e8ea",
	OnBrand:       "#ffffff",
	Success:       "#248a3d",
	SuccessStrong: "#1c6e30",
	SuccessSoft:   "#e2f3e6",
	Warning:       "#b25000",
	WarningStrong: "#8f4008",
	WarningSoft:   "#fbeede",
	Danger:        "#d70015",
	DangerStrong:  "#b00010",
	DangerSoft:    "#fbe5e6",
	DangerHover:   "#f7d3d6",
	Scrim:         "#000000aa",
	Shadow:        "#1b263414",
	ShadowStrong:  "#1b263421",
}

// Of returns the palette for the active appearance; pass
// adw.StyleManager.Dark() (true = dark).
func Of(dark bool) Palette {
	if dark {
		return Dark
	}
	return Light
}

// Metrics (px). GTK works in device-independent px; scale by the surface
// factor if you target hidpi manually (GTK usually handles it).
const SpaceXs = 4
const SpaceSm = 8
const SpaceMd = 12
const SpaceLg = 16
const SpaceXl = 24
const RadiusSm = 6
const RadiusMd = 10
const RadiusPill = 999
const TextXs = 12
const TextSm = 13
const TextMd = 15
const TextLg = 17
