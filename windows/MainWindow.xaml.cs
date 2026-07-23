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
public sealed partial class MainWindow : Window
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
        AppWindow.Resize(new SizeInt32(460, 600));

        // Close hides instead of exiting — the app stays resident in the tray.
        AppWindow.Closing += (_, e) => { e.Cancel = true; AppWindow.Hide(); };

        UrlBox.Text = _agent.ServerUrl;

        _agent.StatusChanged += OnStatus;
        _tick = DispatcherQueue.CreateTimer();
        _tick.Interval = TimeSpan.FromSeconds(1); // let "Controlling now" decay
        _tick.Tick += (_, _) => Render();
        _tick.Start();

        Render();
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
