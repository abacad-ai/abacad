namespace Abacad;

// One activity-trail entry (a command outcome, a state change, a diagnostic).
sealed record ActivityLine(DateTime Ts, string Text);

// Coordinator: owns the socket, the command dispatcher, and the tunnel, and
// bridges them to the tray UI. The command path runs off the UI thread (SendInput/
// UIA/GDI are all callable from a background thread); the tray marshals the
// connection-state event back to the UI thread itself. Mirrors the Agent in
// macos/AbacadApp.swift.
sealed class Agent
{
    public event Action<bool>? ConnectedChanged;
    // Fired on any awareness change (activity, pause, watched, recording). The UI
    // subscribes and marshals a repaint onto the UI thread.
    public event Action? StatusChanged;

    public bool Connected { get; private set; }
    public string ServerUrl { get; private set; } = Prefs.ServerUrl;

    // Awareness state — the consent surface for the person at this PC.
    public bool Paused { get; private set; }
    public bool Watched { get; private set; }
    public bool Recording { get; private set; }
    public string? LastMethod { get; private set; }
    DateTime _lastCommandAt = DateTime.MinValue;

    // True when an agent ran a command in the last few seconds: a distinct
    // "Controlling now" state vs. an idle "Connected". Computed, so the UI polls
    // it on a 1s timer to let it decay.
    public bool Controlling => Connected && !Paused && (DateTime.UtcNow - _lastCommandAt).TotalSeconds < 6;

    readonly List<ActivityLine> _lines = new();
    readonly object _linesLock = new();
    public IReadOnlyList<ActivityLine> Lines
    {
        get { lock (_linesLock) return _lines.ToArray(); }
    }

    readonly WebSocketClient _ws = new();
    readonly CommandDispatcher _dispatcher = new();
    readonly Tunnel _tunnel = new();

    public Agent()
    {
        _dispatcher.Blobs = BlobClient.FromServerUrl(ServerUrl);
        _tunnel.SendFrame = data => _ws.Send(data);
        _ws.OnStateChange = up =>
        {
            Connected = up;
            Event(up ? "• connected" : "• disconnected");
            ConnectedChanged?.Invoke(up);
        };
        _ws.OnText = HandleText;
        _ws.OnBinary = data => _tunnel.Handle(data);
    }

    // Toggle the soft-kill pause (from the settings window). While paused every
    // incoming command is rejected locally; only the window can clear it.
    public void SetPaused(bool p)
    {
        if (Paused == p) return;
        Paused = p;
        Event(p ? "⏸ control paused by device operator" : "▶ control resumed");
    }

    void Event(string text)
    {
        lock (_linesLock)
        {
            _lines.Add(new ActivityLine(DateTime.Now, text));
            if (_lines.Count > 40) _lines.RemoveRange(0, _lines.Count - 40);
        }
        StatusChanged?.Invoke();
    }

    void NoteCommand(string method)
    {
        LastMethod = method;
        _lastCommandAt = DateTime.UtcNow;
        StatusChanged?.Invoke();
    }

    void SetWatched(bool w) { if (Watched != w) { Watched = w; Event(w ? "👁 live view started — screen being watched" : "live view ended"); } }
    void SetRecording(bool r) { if (Recording != r) { Recording = r; Event(r ? "● screen recording started" : "screen recording stopped"); } }

    // Reflect live-view / recording sessions, inferred from the command verbs.
    void UpdateAwareness(string method, Dictionary<string, object?> p)
    {
        var action = p.TryGetValue("action", out var av) ? av as string : null;
        if (method == "vnc") { if (action == "start") SetWatched(true); else if (action == "stop") SetWatched(false); }
        else if (method == "screen_recording") { if (action == "start") SetRecording(true); else if (action == "stop") SetRecording(false); }
    }

    /// Dial the stored server URL on launch, if any.
    public void Start()
    {
        if (!string.IsNullOrEmpty(ServerUrl)) _ws.Connect(ServerUrl);
    }

    public void Connect(string url)
    {
        url = url.Trim();
        Prefs.ServerUrl = url;
        ServerUrl = url;
        // A manual connect is a fresh intent to allow control: clear any pause.
        SetPaused(false);
        // Rebuild the blob endpoint whenever the server URL changes, so file
        // transfer follows the socket to a new host/token.
        _dispatcher.Blobs = BlobClient.FromServerUrl(url);
        _ws.Connect(url);
    }

    public void Disconnect()
    {
        _ws.Disconnect();
        _tunnel.CloseAll();
    }

    // Parse a command frame and dispatch it; reply is correlated by id.
    void HandleText(string text)
    {
        var cmd = Json.Object(text);
        if (cmd is null) return; // malformed → no reply
        string id = cmd.Str("id");
        string method = cmd.Str("method");
        var p = cmd.TryGetValue("params", out var pv) && pv is Dictionary<string, object?> d
            ? d : new Dictionary<string, object?>();

        // Soft-kill: while the operator has paused control, reject every command
        // locally without touching the PC. The agent sees an error; only the
        // settings window clears the pause.
        if (Paused)
        {
            Event($"{method} · rejected · paused");
            _ws.Send(Json.String(new Dictionary<string, object?>
            {
                ["id"] = id, ["ok"] = false, ["error"] = "paused by device operator",
            }));
            return;
        }
        NoteCommand(method);
        UpdateAwareness(method, p);

        _ = Task.Run(async () =>
        {
            try
            {
                var result = await _dispatcher.Execute(method, p);
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = true, ["result"] = result,
                }));
                Event($"{method} · ok");
            }
            catch (Exception e)
            {
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = false, ["error"] = e.Message,
                }));
                Event($"{method} · error · {e.Message}");
            }
        });
    }
}
