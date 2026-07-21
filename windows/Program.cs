namespace Abacad;

// Tray entry point. There is no main window; the only UI is the notification-area
// icon and its small settings panel — the Windows analogue of the macOS menu-bar
// (LSUIElement) app. High-DPI mode comes from app.manifest + ApplicationHighDpiMode.
static class Program
{
    [STAThread]
    static void Main()
    {
        ApplicationConfiguration.Initialize();
        Application.Run(new TrayApp());
    }
}
