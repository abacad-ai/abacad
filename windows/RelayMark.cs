using System.Drawing;
using System.Drawing.Drawing2D;
using System.Runtime.InteropServices;

namespace Abacad;

// abacad relay mark, drawn with GDI+ so the tray icon is our own graph rather than
// a stock glyph. Four connected dots (one hub → three devices) that read as an "A".
// The Windows notification area doesn't auto-tint icons, so connection state is
// shown by color: a green ("alive") hub when connected, a muted gray hub when not.
//
// Keep the geometry in sync with assets/icon.svg: a true 120° equilateral tristar
// from the hub (apex up, feet 120° apart), matching the web / macOS / launcher marks.
static class RelayMark
{
    // Unit-square coordinates, y-down. Hub is the centroid, so it sits a little
    // below visual center (same as macos/RelayMark.swift).
    static readonly PointF Apex = new(0.50f, 0.20f);
    static readonly PointF FootL = new(0.136f, 0.83f);
    static readonly PointF FootR = new(0.864f, 0.83f);
    static readonly PointF Hub = new(0.50f, 0.62f);
    const float NodeR = 0.085f;
    const float HubR = 0.100f;
    const float EdgeW = 0.045f;

    /// Build a tray Icon at the given pixel size. `connected` colors the hub.
    public static Icon Tray(bool connected, int size = 32)
    {
        using var bmp = new Bitmap(size, size);
        using (var g = Graphics.FromImage(bmp))
        {
            g.SmoothingMode = SmoothingMode.AntiAlias;
            g.Clear(Color.Transparent);
            float s = size;
            PointF P(PointF u) => new(u.X * s, u.Y * s);

            // Edges: hub → each device node, half strength so the dots dominate.
            using (var pen = new Pen(Color.FromArgb(128, Theme.Brand), EdgeW * s)
            { StartCap = LineCap.Round, EndCap = LineCap.Round })
                foreach (var foot in new[] { Apex, FootL, FootR })
                    g.DrawLine(pen, P(Hub), P(foot));

            // Device nodes.
            using (var nb = new SolidBrush(Theme.Brand))
                foreach (var foot in new[] { Apex, FootL, FootR })
                    Dot(g, nb, P(foot), NodeR * s);

            // Hub: green when connected, muted otherwise — the one state signal.
            using var hb = new SolidBrush(connected ? Theme.Success : Theme.InkSubtle);
            Dot(g, hb, P(Hub), HubR * s);
        }

        // Icon.FromHandle wraps the HICON; the caller destroys it via Destroy() when
        // done so repeated swaps don't leak GDI handles.
        return Icon.FromHandle(bmp.GetHicon());
    }

    /// Free an Icon created by Tray() (its underlying HICON).
    public static void Destroy(Icon icon) => DestroyIcon(icon.Handle);

    static void Dot(Graphics g, Brush b, PointF c, float r)
        => g.FillEllipse(b, c.X - r, c.Y - r, r * 2, r * 2);

    [DllImport("user32.dll", SetLastError = true)]
    static extern bool DestroyIcon(IntPtr hIcon);
}
