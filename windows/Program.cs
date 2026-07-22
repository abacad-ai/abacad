namespace Abacad;

// Tray entry point. There is no main window; the only UI is the notification-area
// icon and its small settings panel — the Windows analogue of the macOS menu-bar
// (LSUIElement) app. High-DPI mode comes from app.manifest + ApplicationHighDpiMode.
static class Program
{
    [STAThread]
    static int Main(string[] args)
    {
        // `abacad connect` runs the device-authorization pairing flow as a console
        // command and exits; a bare launch runs the tray app. Branch before any
        // WinForms init so connect never spins up the UI.
        if (args.Length > 0 && args[0] == "connect")
            return ConnectFlow.Run(args);

        ApplicationConfiguration.Initialize();
        Application.Run(new TrayApp());
        return 0;
    }
}
