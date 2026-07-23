using System.Drawing;
using System.Text;

namespace Abacad;

// The settings + awareness window — the Windows analogue of the macOS AgentPanel,
// built around the shared consent model. A live State header (Controlling now /
// Connected / Paused / Disconnected), "screen being watched" / "recording" flags,
// a Pause / Disconnect pair, the recent-actions tail, and the server-URL / Connect
// setup (shown when disconnected, hidden once connected). Native WinForms chrome;
// Theme supplies the status colors.
//
// (The client stays on WinForms rather than WinUI 3 because its UI-Automation
// layer — UiTree/CommandDispatcher via UseWPF — cannot coexist with the Windows
// App SDK, and WinForms keeps the cross-OS build the release CI relies on.)
sealed class SettingsForm : Form
{
    readonly Agent _agent;

    readonly Panel _dot;
    readonly Label _title;
    readonly Label _sub;
    readonly Label _watched;
    readonly Label _recording;
    readonly Button _pauseBtn;
    readonly Button _disconnectBtn;
    readonly Label _urlLabel;
    readonly TextBox _url;
    readonly Button _connectBtn;
    readonly Label _actionsLabel;
    readonly TextBox _actions;
    readonly System.Windows.Forms.Timer _tick;

    public SettingsForm(Agent agent)
    {
        _agent = agent;

        Text = "abacad";
        FormBorderStyle = FormBorderStyle.FixedDialog;
        MaximizeBox = false;
        MinimizeBox = false;
        StartPosition = FormStartPosition.CenterScreen;
        ClientSize = new Size(420, 480);
        Font = new Font("Segoe UI", 9f);

        var root = new FlowLayoutPanel
        {
            Dock = DockStyle.Fill,
            FlowDirection = FlowDirection.TopDown,
            WrapContents = false,
            AutoScroll = true,
            Padding = new Padding(16),
        };

        var header = new FlowLayoutPanel { FlowDirection = FlowDirection.LeftToRight, AutoSize = true, WrapContents = false, Margin = new Padding(0) };
        _dot = new Panel { Size = new Size(10, 10), Margin = new Padding(0, 7, 8, 0) };
        _title = new Label { AutoSize = true, Font = new Font("Segoe UI", 12f, FontStyle.Bold) };
        header.Controls.Add(_dot);
        header.Controls.Add(_title);

        _sub = new Label { AutoSize = true, ForeColor = Theme.InkMuted, MaximumSize = new Size(388, 0), Margin = new Padding(0, 0, 0, 4) };
        _watched = new Label { AutoSize = true, ForeColor = Theme.Warning, Font = new Font("Segoe UI", 9f, FontStyle.Bold), Visible = false };
        _recording = new Label { AutoSize = true, ForeColor = Theme.Danger, Font = new Font("Segoe UI", 9f, FontStyle.Bold), Visible = false };

        var btnRow = new FlowLayoutPanel { FlowDirection = FlowDirection.LeftToRight, AutoSize = true, WrapContents = false, Margin = new Padding(0, 6, 0, 0) };
        _pauseBtn = new Button { Text = "Pause", AutoSize = true };
        _pauseBtn.Click += (_, _) => { _agent.SetPaused(!_agent.Paused); Render(); };
        _disconnectBtn = new Button { Text = "Disconnect", AutoSize = true, Margin = new Padding(8, 0, 0, 0), ForeColor = Theme.Danger };
        _disconnectBtn.Click += (_, _) => _agent.Disconnect();
        btnRow.Controls.Add(_pauseBtn);
        btnRow.Controls.Add(_disconnectBtn);

        _urlLabel = new Label { Text = "Server URL", ForeColor = SystemColors.GrayText, AutoSize = true, Margin = new Padding(0, 10, 0, 2) };
        _url = new TextBox { Size = new Size(388, 24), Text = _agent.ServerUrl, PlaceholderText = "wss://host/device?token=…" };
        _connectBtn = new Button { Text = "Connect", AutoSize = true, Margin = new Padding(0, 6, 0, 0) };
        _connectBtn.Click += (_, _) => { var u = _url.Text.Trim(); if (u.Length > 0) _agent.Connect(u); };

        _actionsLabel = new Label { Text = "Recent actions", ForeColor = SystemColors.GrayText, AutoSize = true, Margin = new Padding(0, 10, 0, 2) };
        _actions = new TextBox
        {
            Multiline = true,
            ReadOnly = true,
            ScrollBars = ScrollBars.Vertical,
            Size = new Size(388, 150),
            Font = new Font("Consolas", 8.5f),
            BackColor = SystemColors.Window,
        };

        var note = new Label
        {
            AutoSize = false,
            Size = new Size(388, 56),
            ForeColor = SystemColors.GrayText,
            Margin = new Padding(0, 10, 0, 0),
            Text =
                "Once connected an agent can view and control this PC; Pause rejects commands " +
                "without dropping the link, Disconnect drops it. Driving windows owned by an " +
                "elevated (administrator) app requires running abacad as administrator too.",
        };

        root.Controls.AddRange(new Control[]
        {
            header, _sub, _watched, _recording, btnRow,
            _urlLabel, _url, _connectBtn,
            _actionsLabel, _actions, note,
        });
        Controls.Add(root);

        _agent.StatusChanged += OnStatus;
        _tick = new System.Windows.Forms.Timer { Interval = 1000 };
        _tick.Tick += (_, _) => Render();
        _tick.Start();
        FormClosed += (_, _) => { _agent.StatusChanged -= OnStatus; _tick.Stop(); _tick.Dispose(); };

        Render();
    }

    void OnStatus()
    {
        if (IsHandleCreated && !IsDisposed) BeginInvoke((Action)Render);
    }

    void Render()
    {
        Color dot;
        string title, sub;
        if (_agent.Paused) { dot = Theme.Warning; title = "Paused"; sub = "commands are being rejected on this PC"; }
        else if (_agent.Controlling) { dot = Theme.Success; title = "Controlling now"; sub = $"agent · {_agent.LastMethod ?? "running"}"; }
        else if (_agent.Connected) { dot = Theme.Success; title = "Connected"; sub = "idle — no agent active"; }
        else { dot = Theme.InkSubtle; title = "Disconnected"; sub = "not connected"; }

        _dot.BackColor = dot;
        _title.Text = title;
        _sub.Text = sub;

        _watched.Visible = _agent.Watched;
        _watched.Text = "👁 Screen being watched";
        _recording.Visible = _agent.Recording;
        _recording.Text = "● Recording";

        bool connected = _agent.Connected;
        _pauseBtn.Text = _agent.Paused ? "Resume" : "Pause";
        _pauseBtn.Enabled = connected;
        _disconnectBtn.Enabled = connected;

        // Setup is shown while disconnected and demoted (hidden) once connected;
        // the recent-actions tail is shown only when there's a live connection.
        _urlLabel.Visible = !connected;
        _url.Visible = !connected;
        _connectBtn.Visible = !connected;
        _actionsLabel.Visible = connected;
        _actions.Visible = connected;

        var lines = _agent.Lines;
        if (lines.Count == 0)
        {
            _actions.Text = "No activity yet.";
        }
        else
        {
            var sb = new StringBuilder();
            for (int i = lines.Count - 1; i >= 0 && i >= lines.Count - 14; i--)
                sb.AppendLine($"{lines[i].Ts:HH:mm:ss}  {lines[i].Text}");
            _actions.Text = sb.ToString();
        }
    }

    /// Called by TrayApp (already on the UI thread) on connection change.
    public void SetConnected(bool connected) => Render();
}
