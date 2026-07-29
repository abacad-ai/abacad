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
                    new PopupMenuSeparator(),
                    // What this PC exposes, decided here rather than by the
                    // relay. Grouped, because 21 switches is not a decision
                    // anyone makes well — the same groups the dashboard shows,
                    // so moving between the two means choosing the same things
                    // by the same names. Storage stays per verb.
                    new PopupMenuItem("What this PC exposes:", (_, _) => { }) { Enabled = false },
                    CapItem(0), CapItem(1), CapItem(2), CapItem(3), CapItem(4), CapItem(5),
                    new PopupMenuSeparator(),
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
        // No explicit menu refresh needed: PopupMenu builds the native HMENU from
        // Items on every Show(), so the Text/Enabled set above are picked up at the
        // next right-click.
    }

    // Capability groups mirroring the dashboard's. These are NOT peers: input is
    // close to full control on a desktop, file-write can drop a startup entry,
    // and a tunnel reaches this PC's own ports including sshd — so the labels
    // say so rather than presenting a row of identical-looking switches.
    static readonly (string Label, string[] Members)[] CapGroups =
    {
        ("See the screen", new[] { "screenshot", "screen_recording" }),
        ("Control input (≈ full control)", new[] { "click", "right_click", "drag", "scroll", "press_keys", "input_text", "composite", "tap", "long_press", "swipe" }),
        ("Read and write files (≈ full control)", new[] { "push_file", "pull_file" }),
        ("Live view (also sends input)", new[] { "vnc" }),
        ("Network tunnel (reaches every local port)", new[] { "tunnel" }),
        ("SSH", new[] { "ssh" }),
    };

    // One toggle row. Flat PopupMenuItems rather than a submenu: this file
    // already proves that type works, and a checkmark in the label shows state
    // without depending on a menu API this build cannot be compiled against here.
    static PopupMenuItem CapItem(int index)
    {
        var (label, members) = CapGroups[index];
        var item = new PopupMenuItem(CapLabel(label, members), (_, _) =>
        {
            var want = !members.All(Capabilities.Allows);
            foreach (var c in members) Capabilities.Toggle(c, want);
        });
        // Repaint the checkmark when the set changes from anywhere.
        Capabilities.OnChange(() => item.Text = CapLabel(label, members));
        return item;
    }

    static string CapLabel(string label, string[] members) =>
        (members.All(Capabilities.Allows) ? "✓ " : "     ") + label;

    public void Dispose()
    {
        _tray.Dispose();
        RelayMark.Destroy(_iconOn);
        RelayMark.Destroy(_iconOff);
    }
}
