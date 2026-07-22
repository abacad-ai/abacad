namespace Abacad;

// Coordinator: owns the socket, the command dispatcher, and the tunnel, and
// bridges them to the tray UI. The command path runs off the UI thread (SendInput/
// UIA/GDI are all callable from a background thread); the tray marshals the
// connection-state event back to the UI thread itself. Mirrors the Agent in
// macos/AbacadApp.swift.
sealed class Agent
{
    public event Action<bool>? ConnectedChanged;
    public bool Connected { get; private set; }
    public string ServerUrl { get; private set; } = Prefs.ServerUrl;

    readonly WebSocketClient _ws = new();
    readonly CommandDispatcher _dispatcher = new();
    readonly Tunnel _tunnel = new();

    public Agent()
    {
        _dispatcher.Blobs = BlobClient.FromServerUrl(ServerUrl);
        _tunnel.SendFrame = data => _ws.Send(data);
        _ws.OnStateChange = up => { Connected = up; ConnectedChanged?.Invoke(up); };
        _ws.OnText = HandleText;
        _ws.OnBinary = data => _tunnel.Handle(data);
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

        _ = Task.Run(async () =>
        {
            try
            {
                var result = await _dispatcher.Execute(method, p);
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = true, ["result"] = result,
                }));
            }
            catch (Exception e)
            {
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = false, ["error"] = e.Message,
                }));
            }
        });
    }
}
