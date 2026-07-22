using System.Drawing;
using System.Drawing.Imaging;
using System.Runtime.InteropServices;

namespace Abacad;

// Captures the primary display with GDI (Graphics.CopyFromScreen → BitBlt) and
// encodes to JPEG. Because the process is PerMonitorV2 DPI-aware, the capture is
// in physical pixels, matching the coordinate space used by the UIA tree bounds
// and SendInput — the Android invariant "1 screenshot pixel == 1 click unit".
// Field name stays `png_base64` for wire compatibility even though the bytes are
// JPEG (mirrors macos/ScreenCapture.swift). Primary display only in v0.
static class ScreenCapture
{
    public readonly record struct Shot(int W, int H, string Base64);

    static readonly ImageCodecInfo JpegCodec =
        ImageCodecInfo.GetImageEncoders().First(c => c.FormatID == ImageFormat.Jpeg.Guid);

    public static Shot Capture(long jpegQuality = 70)
    {
        int w = Math.Max(1, GetSystemMetrics(SM_CXSCREEN)); // primary display, physical px
        int h = Math.Max(1, GetSystemMetrics(SM_CYSCREEN));

        using var bmp = new Bitmap(w, h, PixelFormat.Format32bppArgb);
        using (var g = Graphics.FromImage(bmp))
            g.CopyFromScreen(0, 0, 0, 0, new Size(w, h), CopyPixelOperation.SourceCopy);

        using var ms = new MemoryStream();
        using (var ep = new EncoderParameters(1))
        {
            ep.Param[0] = new EncoderParameter(Encoder.Quality, jpegQuality);
            bmp.Save(ms, JpegCodec, ep);
        }
        return new Shot(w, h, System.Convert.ToBase64String(ms.GetBuffer(), 0, (int)ms.Length));
    }

    // Capture the primary display as raw 32-bit BGRX pixels (bytes B,G,R,X per
    // pixel) — the format the VNC server sends as a Raw rectangle.
    public static (int W, int H, byte[] Pixels) CaptureRawBGRA()
    {
        int w = Math.Max(1, GetSystemMetrics(SM_CXSCREEN));
        int h = Math.Max(1, GetSystemMetrics(SM_CYSCREEN));

        using var bmp = new Bitmap(w, h, PixelFormat.Format32bppArgb);
        using (var g = Graphics.FromImage(bmp))
            g.CopyFromScreen(0, 0, 0, 0, new Size(w, h), CopyPixelOperation.SourceCopy);

        // Format32bppArgb is B,G,R,A in memory (little-endian) — exactly the BGRX
        // bytes the RFB Raw rect wants (alpha byte ignored by the pixel format).
        var data = bmp.LockBits(new Rectangle(0, 0, w, h), ImageLockMode.ReadOnly, PixelFormat.Format32bppArgb);
        var pixels = new byte[w * h * 4];
        try
        {
            for (int y = 0; y < h; y++)
                Marshal.Copy(data.Scan0 + y * data.Stride, pixels, y * w * 4, w * 4);
        }
        finally
        {
            bmp.UnlockBits(data);
        }
        return (w, h, pixels);
    }

    const int SM_CXSCREEN = 0, SM_CYSCREEN = 1;

    [DllImport("user32.dll")]
    static extern int GetSystemMetrics(int nIndex);
}
