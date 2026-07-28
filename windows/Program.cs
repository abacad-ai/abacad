using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;

namespace Abacad;

// Entry point. There is no persistent main window by default; the app lives in the
// notification area (tray) and shows its Fluent settings/awareness window on
// demand — the Windows analogue of the macOS menu-bar (LSUIElement) app.
//
// A custom Main (DisableXamlGeneratedMain) is used so `abacad connect` can run the
// device-authorization pairing CLI and exit before any WinUI initialization.
static class Program
{
    [STAThread]
    static int Main(string[] args)
    {
        if (args.Length > 0 && args[0] == "connect")
            return ConnectFlow.Run(args);

        WinRT.ComWrappersSupport.InitializeComWrappers();
        // Parameter is named `p`, not `_`: a `_` parameter is a real name here, so
        // the `_ = new App()` discard below would bind to it and fail to compile.
        Application.Start(p =>
        {
            var context = new DispatcherQueueSynchronizationContext(DispatcherQueue.GetForCurrentThread());
            SynchronizationContext.SetSynchronizationContext(context);
            _ = new App();
        });
        return 0;
    }
}
