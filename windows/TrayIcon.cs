using System.Drawing;
using H.NotifyIcon.Core;

namespace Abacad;

// Notification-area icon for the WinUI app. WinUI 3 has no NotifyIcon, so this
// uses H.NotifyIcon.Core — a XAML-free Shell_NotifyIcon wrapper that takes an
// HICON directly, letting us reuse RelayMark's GDI-drawn glyph. Reflects the
// awareness state (connected / controlling / paused) in the tooltip + status
// item, and offers Pause/Resume, Settings, Disconnect, Quit.
sealed class TrayIcon : IDisposable
{
    readonly Agent _agent;
    readonly Action _showWindow;
    readonly Action _quit;

    readonly Icon _iconOn;
    readonly Icon _iconOff;
    readonly TrayIconWithContextMenu _tray;
    readonly PopupMenuItem _statusItem;
    readonly PopupMenuItem _pauseItem;

    public TrayIcon(Agent agent, Action showWindow, Action quit)
    {
        _agent = agent;
        _showWindow = showWindow;
        _quit = quit;

        _iconOn = RelayMark.Tray(connected: true);
        _iconOff = RelayMark.Tray(connected: false);

        _statusItem = new PopupMenuItem("Disconnected", (_, _) => { }) { Enabled = false };
        _pauseItem = new PopupMenuItem("Pause control", (_, _) => _agent.SetPaused(!_agent.Paused));

        _tray = new TrayIconWithContextMenu
        {
            Icon = _iconOff.Handle,
            ToolTip = "abacad — disconnected",
            ContextMenu = new PopupMenu
            {
                Items =
                {
                    _statusItem,
                    new PopupMenuSeparator(),
                    _pauseItem,
                    new PopupMenuItem("Settings…", (_, _) => _showWindow()),
                    new PopupMenuItem("Disconnect", (_, _) => _agent.Disconnect()),
                    new PopupMenuSeparator(),
                    new PopupMenuItem("Quit abacad", (_, _) => _quit()),
                },
            },
        };
        _tray.MessageWindow.MouseEventReceived += (_, e) =>
        {
            if (e.MouseEvent == MouseEvent.IconDoubleClick) _showWindow();
        };
        _tray.Create();

        _agent.ConnectedChanged += _ => ApplyStatus();
        _agent.StatusChanged += ApplyStatus;
        ApplyStatus();
    }

    void ApplyStatus()
    {
        _tray.UpdateIcon(_agent.Connected ? _iconOn.Handle : _iconOff.Handle);

        string label =
            _agent.Paused ? "Paused — commands rejected"
            : _agent.Controlling ? "Controlling now"
            : _agent.Connected ? "Connected — viewable & controllable remotely"
            : "Disconnected";
        _statusItem.Text = label;
        _pauseItem.Text = _agent.Paused ? "Resume control" : "Pause control";
        _pauseItem.Enabled = _agent.Connected;

        _tray.ToolTip = "abacad — " +
            (_agent.Paused ? "paused" : _agent.Controlling ? "controlling now" : _agent.Connected ? "connected" : "disconnected");
        try { _tray.UpdateContextMenu(); } catch { /* best-effort menu refresh */ }
    }

    public void Dispose()
    {
        _tray.Dispose();
        RelayMark.Destroy(_iconOn);
        RelayMark.Destroy(_iconOff);
    }
}
