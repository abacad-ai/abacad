using System.Runtime.InteropServices;
using Microsoft.UI;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;
using Windows.Graphics;
using Windows.UI;

namespace Abacad;

// The settings + awareness window — the WinUI 3 / Fluent analogue of the macOS
// AgentPanel. A live State header (Controlling now / Connected / Paused /
// Disconnected), "screen being watched" / "recording" flags, a Pause / Disconnect
// pair, the recent-actions tail, and the server-URL / Connect setup (shown while
// disconnected). Closing hides the window; the app exits only from the tray.
// internal, not public: its constructor takes Agent (and it surfaces
// ActivityLine), both of which are internal — a public MainWindow would be
// inconsistently accessible (CS0051). Only App holds one, in a private field.
internal sealed partial class MainWindow : Window
{
    readonly Agent _agent;
    readonly DispatcherQueueTimer _tick;

    // Status colors from design/tokens.json (dark set); the neutral chrome comes
    // from Theme.xaml ThemeResources + Mica.
    static readonly Color Success = Color.FromArgb(255, 48, 209, 88);
    static readonly Color Warning = Color.FromArgb(255, 255, 159, 10);
    static readonly Color Danger = Color.FromArgb(255, 255, 69, 58);
    static readonly Color InkSubtle = Color.FromArgb(255, 102, 102, 108);
    static readonly Color WarningSoft = Color.FromArgb(255, 46, 33, 9);
    static readonly Color DangerSoft = Color.FromArgb(255, 47, 18, 16);

    public MainWindow(Agent agent)
    {
        _agent = agent;
        InitializeComponent();

        SystemBackdrop = new MicaBackdrop();
        Title = "abacad";
        SizeInDips(460, 600);
        ApplyWindowIcon();

        // Close hides instead of exiting — the app stays resident in the tray.
        AppWindow.Closing += (_, e) => { e.Cancel = true; AppWindow.Hide(); };

        UrlBox.Text = _agent.ServerUrl;
        RelayBox.Text = _agent.RelayUrl;

        _agent.StatusChanged += OnStatus;
        _agent.EnrollmentChanged += OnStatus;
        _tick = DispatcherQueue.CreateTimer();
        _tick.Interval = TimeSpan.FromSeconds(1); // let "Controlling now" decay
        _tick.Tick += (_, _) => Render();
        _tick.Start();

        Render();
    }

    [DllImport("user32.dll")]
    static extern uint GetDpiForWindow(IntPtr hwnd);

    // AppWindow.Resize takes PHYSICAL pixels, but every size in MainWindow.xaml is
    // a DIP. Those coincide only at 100% scaling, so a hard-coded 460x600 gave a
    // 153x200 DIP window on the 4K/300% display this was first run on — narrower
    // than the 230-DIP header column alone, which is why the state line clipped
    // mid-word. Scale by the window's own DPI so the request means what it says.
    void SizeInDips(int width, int height)
    {
        var hwnd = WinRT.Interop.WindowNative.GetWindowHandle(this);
        double scale = GetDpiForWindow(hwnd) / 96.0;
        if (scale <= 0) scale = 1; // GetDpiForWindow returns 0 on failure
        AppWindow.Resize(new SizeInt32(
            (int)Math.Round(width * scale), (int)Math.Round(height * scale)));
    }

    // Held deliberately: AppWindow keeps using the HICON, so letting the managed
    // Icon finalise would destroy it out from under the title bar and Alt-Tab.
    static System.Drawing.Icon? _appIcon;

    // An unpackaged WinUI 3 window does not pick up the exe's icon resource the way
    // a WinForms or WPF one does, which is why the title bar showed the generic
    // executable glyph. ApplicationIcon (see the csproj) already embeds Abacad.ico
    // into the exe, so read it back out of our own image — a single-file bundle has
    // nowhere to keep a loose .ico for AppWindow.SetIcon(string) to point at.
    void ApplyWindowIcon()
    {
        try
        {
            _appIcon ??= System.Drawing.Icon.ExtractAssociatedIcon(Environment.ProcessPath!);
            if (_appIcon != null)
                AppWindow.SetIcon(Win32Interop.GetIconIdFromIcon(_appIcon.Handle));
        }
        catch
        {
            // A missing icon is cosmetic and must never stop the window opening —
            // an exception here would be thrown on the same path that v0.5.2 died on.
        }
    }

    public void BringToFront()
    {
        AppWindow.Show();
        Activate();
    }

    void OnStatus() => DispatcherQueue.TryEnqueue(Render);

    void OnPause(object sender, RoutedEventArgs e) { _agent.SetPaused(!_agent.Paused); Render(); }
    void OnDisconnect(object sender, RoutedEventArgs e) => _agent.Disconnect();
    void OnConnect(object sender, RoutedEventArgs e)
    {
        var u = UrlBox.Text.Trim();
        if (u.Length > 0) _agent.Connect(u);
    }

    void OnForget(object sender, RoutedEventArgs e) { _agent.ForgetEnrollment(); Render(); }

    // The only path on a fresh install that is allowed to contact a relay.
    void OnSetup(object sender, RoutedEventArgs e)
    {
        _agent.StartEnrollment(allowRegister: true);
        Render();
    }

    void OnChangeRelay(object sender, RoutedEventArgs e)
    {
        var r = RelayBox.Text.Trim();
        if (r.Length > 0) _agent.ChangeRelay(r);
        Render();
    }

    void Render()
    {
        Color dot;
        string title, sub;
        if (_agent.Paused) { dot = Warning; title = "Paused"; sub = "commands are being rejected on this PC"; }
        else if (_agent.Controlling) { dot = Success; title = "Controlling now"; sub = $"agent · {_agent.LastMethod ?? "running"}"; }
        else if (_agent.Connected) { dot = Success; title = "Connected"; sub = "idle — no agent active"; }
        else { dot = InkSubtle; title = "Disconnected"; sub = "not connected"; }

        Dot.Fill = new SolidColorBrush(dot);
        TitleText.Text = title;
        SubtitleText.Text = sub;

        // watched / recording pills
        Flags.Children.Clear();
        if (_agent.Watched) Flags.Children.Add(Pill("👁 Screen being watched", Warning, WarningSoft));
        if (_agent.Recording) Flags.Children.Add(Pill("● Recording", Danger, DangerSoft));

        bool connected = _agent.Connected;
        PauseBtn.Content = _agent.Paused ? "Resume" : "Pause";
        PauseBtn.IsEnabled = connected;
        DisconnectBtn.IsEnabled = connected;
        ControlButtons.Visibility = connected ? Visibility.Visible : Visibility.Collapsed;

        // setup while disconnected; the recent-actions tail while connected
        SetupPanel.Visibility = connected ? Visibility.Collapsed : Visibility.Visible;
        ActionsPanel.Visibility = connected ? Visibility.Visible : Visibility.Collapsed;

        // Self-enrollment: show the two lines a human reads off this PC while a
        // claim code is live, and who claimed it once one has.
        var relay = Enrollment.Normalize(_agent.RelayUrl);

        // First-run prompt. Every panel in SetupPanel defaults to Visible in the
        // XAML, so this has to be set on both branches or it lingers.
        // Shown when nothing has been set up, and also after a failure on a PC
        // that already holds a token — otherwise that case renders an error with
        // no way to act on it.
        bool needsSetup = (_agent.NeedsSetup || _agent.EnrollError != null) && !_agent.Enrolling;
        NeedsSetupPanel.Visibility = needsSetup ? Visibility.Visible : Visibility.Collapsed;
        if (needsSetup)
        {
            bool retry = !_agent.NeedsSetup;
            SetupPromptText.Text = retry ? "Setup didn't finish." : "This PC hasn't been set up yet.";
            SetupButton.Content = retry ? "Try again" : "Set up this PC";
            SetupHintText.Text = retry
                ? ""
                : $"Registers with {relay} and shows a code to enter there. Nothing is sent until you press it.";
            SetupHintText.Visibility = retry ? Visibility.Collapsed : Visibility.Visible;
        }

        bool showClaim = _agent.ClaimCode.Length > 0;
        ClaimPanel.Visibility = showClaim ? Visibility.Visible : Visibility.Collapsed;
        if (showClaim)
        {
            DeviceIdText.Text = _agent.DeviceId;
            ClaimCodeText.Text = _agent.ClaimCode;
            ClaimHintText.Text = $"Open {relay}/claim and enter both.";
        }
        EnrollErrorText.Text = _agent.EnrollError ?? "";
        EnrollErrorText.Visibility =
            _agent.EnrollError == null ? Visibility.Collapsed : Visibility.Visible;
        ClaimedPanel.Visibility =
            _agent.ClaimedBy.Length > 0 ? Visibility.Visible : Visibility.Collapsed;
        ClaimedByText.Text = $"Claimed by {_agent.ClaimedBy}";
        // Telling someone already on their own relay to run their own relay is noise.
        SelfHostHint.Visibility =
            relay == Enrollment.DefaultRelay ? Visibility.Visible : Visibility.Collapsed;

        var lines = _agent.Lines;
        if (lines.Count == 0)
        {
            ActionsText.Text = "No activity yet.";
        }
        else
        {
            var sb = new System.Text.StringBuilder();
            for (int i = lines.Count - 1; i >= 0 && i >= lines.Count - 14; i--)
                sb.AppendLine($"{lines[i].Ts:HH:mm:ss}  {lines[i].Text}");
            ActionsText.Text = sb.ToString();
        }
    }

    static Border Pill(string text, Color fg, Color bg) => new()
    {
        Background = new SolidColorBrush(bg),
        CornerRadius = new CornerRadius(999),
        Padding = new Thickness(10, 4, 10, 4),
        Child = new TextBlock { Text = text, FontSize = 12, FontWeight = Microsoft.UI.Text.FontWeights.SemiBold, Foreground = new SolidColorBrush(fg) },
    };
}
