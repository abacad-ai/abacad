using System.Drawing;

namespace Abacad;

// The settings panel: connection status, the server URL, and Connect/Disconnect —
// the Windows analogue of the macOS AgentPanel. Windows needs no per-capability
// grant like macOS TCC, so instead of permission rows it shows a short note about
// the one real limit (driving elevated windows needs an elevated client).
sealed class SettingsForm : Form
{
    readonly Agent _agent;
    readonly Label _statusDot;
    readonly Label _statusText;
    readonly TextBox _url;

    public SettingsForm(Agent agent)
    {
        _agent = agent;

        Text = "abacad";
        FormBorderStyle = FormBorderStyle.FixedDialog;
        MaximizeBox = false;
        MinimizeBox = false;
        StartPosition = FormStartPosition.CenterScreen;
        ClientSize = new Size(420, 250);
        Font = new Font("Segoe UI", 9f);

        bool connected = _agent.Connected;

        _statusDot = new Label
        {
            AutoSize = false,
            Size = new Size(10, 10),
            Location = new Point(16, 20),
            BackColor = connected ? Theme.Success : Theme.InkSubtle,
        };
        _statusText = new Label
        {
            AutoSize = true,
            Location = new Point(34, 15),
            Font = new Font("Segoe UI", 11f, FontStyle.Bold),
            Text = connected ? "Connected" : "Disconnected",
        };

        var urlLabel = new Label
        {
            AutoSize = true,
            Location = new Point(16, 56),
            ForeColor = SystemColors.GrayText,
            Text = "Server URL",
        };
        _url = new TextBox
        {
            Location = new Point(16, 76),
            Size = new Size(388, 24),
            Text = _agent.ServerUrl,
            PlaceholderText = "wss://host/device?token=…",
        };

        var connectBtn = new Button
        {
            Location = new Point(16, 110),
            Size = new Size(110, 30),
            Text = "Connect",
        };
        connectBtn.Click += (_, _) =>
        {
            var u = _url.Text.Trim();
            if (u.Length > 0) _agent.Connect(u);
        };

        var disconnectBtn = new Button
        {
            Location = new Point(134, 110),
            Size = new Size(110, 30),
            Text = "Disconnect",
        };
        disconnectBtn.Click += (_, _) => _agent.Disconnect();

        var note = new Label
        {
            AutoSize = false,
            Location = new Point(16, 158),
            Size = new Size(388, 76),
            ForeColor = SystemColors.GrayText,
            Text =
                "Provision a Windows device on the server, then paste its wss://…/device?token=… URL above.\n\n" +
                "Note: driving windows owned by an elevated (administrator) app requires running abacad as " +
                "administrator too. Capture and input target the primary display.",
        };

        Controls.AddRange(new Control[]
        {
            _statusDot, _statusText, urlLabel, _url, connectBtn, disconnectBtn, note,
        });
    }

    /// Called by TrayApp (already marshaled to the UI thread) on state change.
    public void SetConnected(bool connected)
    {
        if (IsDisposed) return;
        _statusDot.BackColor = connected ? Theme.Success : Theme.InkSubtle;
        _statusText.Text = connected ? "Connected" : "Disconnected";
    }
}
