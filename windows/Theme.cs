using System.Drawing;

namespace Abacad;

// abacad design tokens (dark variant of design/tokens.json). The tray glyph and
// settings panel keep native Windows chrome; these supply the shared semantic
// accents (status green/amber/red, brand) so the app reads as the same product as
// the dashboard and the macOS client. Values mirror macos/Theme.swift's dark set.
static class Theme
{
    static Color Rgb(int r, int g, int b) => Color.FromArgb(r, g, b);

    public static readonly Color Canvas = Rgb(11, 11, 13);
    public static readonly Color Surface = Rgb(22, 22, 24);
    public static readonly Color Border = Rgb(42, 42, 46);
    public static readonly Color Ink = Rgb(242, 242, 244);
    public static readonly Color InkMuted = Rgb(154, 154, 160);
    public static readonly Color InkSubtle = Rgb(102, 102, 108);
    public static readonly Color Brand = Rgb(216, 218, 222); // #d8dade — the relay device nodes
    public static readonly Color Success = Rgb(48, 209, 88);  // #30d158 — the "alive" hub
    public static readonly Color Warning = Rgb(255, 159, 10);
    public static readonly Color Danger = Rgb(255, 69, 58);
}
