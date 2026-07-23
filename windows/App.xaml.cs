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

    public App()
    {
        InitializeComponent();
    }

    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        _agent = new Agent();
        _tray = new TrayIcon(_agent, ShowWindow, Quit);
        _agent.Start();
    }

    void ShowWindow()
    {
        _window ??= new MainWindow(_agent);
        _window.Activate();
        _window.BringToFront();
    }

    void Quit()
    {
        _agent.Disconnect();
        _tray.Dispose();
        _window?.Close();
        Exit();
    }
}
