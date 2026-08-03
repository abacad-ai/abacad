using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;

namespace Abacad;

// The WinUI application. Owns the single Agent, the tray icon, and the (hidden
// until shown) settings/awareness window. Closing the window hides it; the app
// exits only from the tray's Quit item.
public partial class App : Application
{
    Agent _agent = null!;
    TrayIcon _tray = null!;
    MainWindow? _window;

    // The UI thread's queue, captured in OnLaunched (which runs on it).
    //
    // Load-bearing: H.NotifyIcon pumps its message window on its OWN thread, so
    // every tray menu callback arrives off the UI thread. Touching XAML there
    // throws RPC_E_WRONG_THREAD (0x8001010E) — and because the callback is
    // running inside the native TrackPopupMenuEx loop, the CLR cannot unwind the
    // exception out of it. The process is killed on the spot with
    // STATUS_FATAL_USER_CALLBACK_EXCEPTION (0xC000041D): no error dialog, no
    // managed handler, the tray icon just vanishes. That was the v0.5.2 crash on
    // "Settings…" — every XAML touch below must be marshalled.
    DispatcherQueue _ui = null!;

    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        _ui = DispatcherQueue.GetForCurrentThread();
        _agent = new Agent();
        _tray = new TrayIcon(_agent, ShowWindow, Quit);
        _agent.Start();

        // Start() no longer registers on its own, so on a fresh install the app
        // would otherwise sit in the tray doing nothing visible and the setup
        // button would be two clicks away behind "Settings…". Show the window so
        // the first run is self-explanatory. Only while un-set-up: once a token
        // exists this never fires again, and the installer's autostart entry
        // stays as silent at sign-in as it is today.
        if (_agent.NeedsSetup) ShowWindow();
    }

    // Constructing a Window is itself a XAML call, so the hop has to happen
    // before the ??=, not inside MainWindow.
    void ShowWindow()
    {
        if (!_ui.HasThreadAccess) { _ui.TryEnqueue(ShowWindow); return; }

        _window ??= new MainWindow(_agent);
        _window.Activate();
        _window.BringToFront();
    }

    // Same hop, and for the same reason: Close() and Exit() are both XAML calls.
    // Enqueueing also lets the tray's menu loop unwind before Dispose tears the
    // message window down, rather than disposing it from inside its own callback.
    void Quit()
    {
        if (!_ui.HasThreadAccess) { _ui.TryEnqueue(Quit); return; }

        _agent.Disconnect();
        _tray.Dispose();
        _window?.Close();
        Exit();
    }
}
