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
    readonly ToolStripMenuItem _pauseItem;
    readonly Control _marshal = new();   // forces a UI-thread handle for BeginInvoke
    readonly System.Windows.Forms.Timer _poll;   // lets the "Controlling now" state decay
    SettingsForm? _settings;

    public TrayApp()
    {
        _ = _marshal.Handle; // realize the handle on the UI thread

        _iconOn = RelayMark.Tray(connected: true);
        _iconOff = RelayMark.Tray(connected: false);

        _statusItem = new ToolStripMenuItem("Disconnected") { Enabled = false };
        _pauseItem = new ToolStripMenuItem("Pause control", null, (_, _) => _agent.SetPaused(!_agent.Paused));
        var menu = new ContextMenuStrip();
        menu.Items.Add(_statusItem);
        menu.Items.Add(new ToolStripSeparator());
        menu.Items.Add(_pauseItem);
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
        _agent.StatusChanged += OnStatusChanged;
        _poll = new System.Windows.Forms.Timer { Interval = 1000 };
        _poll.Tick += (_, _) => ApplyStatus();
        _poll.Start();
        _agent.Start();
    }

    void OnConnectedChanged(bool connected)
    {
        // The event may fire from a socket background thread; marshal to the UI.
        if (_marshal.IsHandleCreated)
            _marshal.BeginInvoke(() => ApplyState(connected));
    }

    void OnStatusChanged()
    {
        if (_marshal.IsHandleCreated) _marshal.BeginInvoke((Action)ApplyStatus);
    }

    void ApplyState(bool connected)
    {
        _tray.Icon = connected ? _iconOn : _iconOff;
        _settings?.SetConnected(connected);
        ApplyStatus();
    }

    // Reflect the awareness state in the tray text + menu: a paused, actively
    // controlling, connected, or disconnected device each read differently, and
    // the Pause item flips to Resume while paused.
    void ApplyStatus()
    {
        string label =
            _agent.Paused ? "Paused — commands rejected"
            : _agent.Controlling ? "Controlling now"
            : _agent.Connected ? "Connected — viewable & controllable remotely"
            : "Disconnected";
        _statusItem.Text = label;
        // NotifyIcon.Text is capped at 63 chars; these are well under.
        _tray.Text = "abacad — " +
            (_agent.Paused ? "paused" : _agent.Controlling ? "controlling now" : _agent.Connected ? "connected" : "disconnected");
        _pauseItem.Text = _agent.Paused ? "Resume control" : "Pause control";
        _pauseItem.Enabled = _agent.Connected;
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
            _poll.Stop();
            _poll.Dispose();
            _tray.Dispose();
            RelayMark.Destroy(_iconOn);
            RelayMark.Destroy(_iconOff);
            _marshal.Dispose();
        }
        base.Dispose(disposing);
    }
}
