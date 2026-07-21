using System.ComponentModel;

namespace Abacad;

// Notification-area entry point. No main window — the only UI is the tray icon, its
// context menu, and a small settings panel. The Windows counterpart of the macOS
// MenuBarExtra. Owns the Agent and reflects its connection state in the icon + text.
sealed class TrayApp : ApplicationContext
{
    readonly Agent _agent = new();
    readonly NotifyIcon _tray;
    readonly Icon _iconOn;
    readonly Icon _iconOff;
    readonly ToolStripMenuItem _statusItem;
    readonly Control _marshal = new();   // forces a UI-thread handle for BeginInvoke
    SettingsForm? _settings;

    public TrayApp()
    {
        _ = _marshal.Handle; // realize the handle on the UI thread

        _iconOn = RelayMark.Tray(connected: true);
        _iconOff = RelayMark.Tray(connected: false);

        _statusItem = new ToolStripMenuItem("Disconnected") { Enabled = false };
        var menu = new ContextMenuStrip();
        menu.Items.Add(_statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Settings…", null, (_, _) => ShowSettings());
        menu.Items.Add("Disconnect", null, (_, _) => _agent.Disconnect());
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add("Quit abacad", null, (_, _) => Quit());

        _tray = new NotifyIcon
        {
            Icon = _iconOff,
            Text = "abacad — disconnected",
            Visible = true,
            ContextMenuStrip = menu,
        };
        _tray.DoubleClick += (_, _) => ShowSettings();

        _agent.ConnectedChanged += OnConnectedChanged;
        _agent.Start();
    }

    void OnConnectedChanged(bool connected)
    {
        // The event may fire from a socket background thread; marshal to the UI.
        if (_marshal.IsHandleCreated)
            _marshal.BeginInvoke(() => ApplyState(connected));
    }

    void ApplyState(bool connected)
    {
        _tray.Icon = connected ? _iconOn : _iconOff;
        // NotifyIcon.Text is capped at 63 chars; these are well under.
        _tray.Text = connected ? "abacad — connected" : "abacad — disconnected";
        _statusItem.Text = connected ? "Connected" : "Disconnected";
        _settings?.SetConnected(connected);
    }

    void ShowSettings()
    {
        if (_settings is { IsDisposed: false })
        {
            _settings.Activate();
            return;
        }
        _settings = new SettingsForm(_agent);
        _settings.FormClosed += (_, _) => _settings = null;
        _settings.Show();
        _settings.Activate();
    }

    void Quit()
    {
        _tray.Visible = false;
        _agent.Disconnect();
        ExitThread();
    }

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _tray.Dispose();
            RelayMark.Destroy(_iconOn);
            RelayMark.Destroy(_iconOff);
            _marshal.Dispose();
        }
        base.Dispose(disposing);
    }
}
