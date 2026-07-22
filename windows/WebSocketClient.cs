using System.Net.WebSockets;
using System.Reflection;
using System.Text;

namespace Abacad;

// Outbound WebSocket to the abacad relay's /device endpoint. The PC dials out
// (NAT-friendly; the server never connects in). Text frames carry the JSON
// command/reply lane; binary frames carry the tunnel lane. Auto-reconnects with
// exponential backoff, and the built-in keep-alive ping keeps the idle socket
// alive. Mirrors macos/WebSocketClient.swift.
sealed class WebSocketClient
{
    public Action<string>? OnText;
    public Action<byte[]>? OnBinary;
    public Action<bool>? OnStateChange;

    readonly object _gate = new();
    readonly SemaphoreSlim _sendLock = new(1, 1);
    Uri? _url;
    string? _token;
    bool _closedByUser;
    int _backoff = 1;
    ClientWebSocket? _ws;
    CancellationTokenSource? _cts;

    bool _connected;
    bool Connected
    {
        get => _connected;
        set { if (_connected != value) { _connected = value; OnStateChange?.Invoke(value); } }
    }

    public void Connect(string urlString)
    {
        // URL(string:) accepts junk; only ws/wss are valid WebSocket schemes.
        if (!Uri.TryCreate(urlString.Trim(), UriKind.Absolute, out var u)) return;
        var scheme = u.Scheme.ToLowerInvariant();
        if (scheme != "ws" && scheme != "wss") return;
        // Refuse a plaintext control channel to anything but loopback: this PC
        // carries screen contents + input injection, so a cleartext hop is a full
        // MITM. Real servers must use wss://.
        if (scheme == "ws" && !IsLoopback(u.Host)) return;

        // Carry the device token in the Authorization header, not the URL, so it
        // stays out of server/proxy access logs. Strip any ?token= and migrate it.
        var (stripped, token) = StripToken(u);

        // Restart cleanly if a previous loop is running.
        CancellationTokenSource? old;
        lock (_gate) { old = _cts; _closedByUser = false; _url = stripped; _token = token; }
        old?.Cancel();

        _ = Task.Run(RunLoop);
    }

    public void Disconnect()
    {
        ClientWebSocket? ws;
        CancellationTokenSource? cts;
        lock (_gate) { _closedByUser = true; ws = _ws; cts = _cts; }
        cts?.Cancel();
        try { ws?.Abort(); } catch { }
        Connected = false;
    }

    public void Send(string text)
    {
        var ws = Volatile.Read(ref _ws);
        if (ws is null) return;
        _ = SendRaw(ws, Encoding.UTF8.GetBytes(text), WebSocketMessageType.Text);
    }

    public void Send(byte[] data)
    {
        var ws = Volatile.Read(ref _ws);
        if (ws is null) return;
        _ = SendRaw(ws, data, WebSocketMessageType.Binary);
    }

    async Task RunLoop()
    {
        while (true)
        {
            Uri? url;
            string? token;
            lock (_gate) { if (_closedByUser) return; url = _url; token = _token; }
            if (url is null) return;

            var ws = new ClientWebSocket();
            ws.Options.KeepAliveInterval = TimeSpan.FromSeconds(20);
            if (token is not null)
                ws.Options.SetRequestHeader("Authorization", $"Bearer {token}");
            var cts = new CancellationTokenSource();
            lock (_gate) { _ws = ws; _cts = cts; }

            try
            {
                await ws.ConnectAsync(url, cts.Token);
                _backoff = 1;
                Connected = true;
                await Receive(ws, cts.Token);
            }
            catch { /* fall through to reconnect */ }

            Connected = false;
            lock (_gate) { if (ReferenceEquals(_ws, ws)) _ws = null; }
            try { ws.Dispose(); } catch { }

            bool done;
            int delay;
            lock (_gate)
            {
                done = _closedByUser;
                delay = _backoff;
                _backoff = Math.Min(_backoff * 2, 15); // cap at 15s, matching the other clients
            }
            if (done) return;
            try { await Task.Delay(delay * 1000, cts.Token); } catch { }
        }
    }

    async Task Receive(ClientWebSocket ws, CancellationToken ct)
    {
        // Relay screenshots are multi-MB base64; accumulate fragments until the
        // frame is complete rather than capping the message size.
        var seg = new ArraySegment<byte>(new byte[64 * 1024]);
        while (ws.State == WebSocketState.Open && !ct.IsCancellationRequested)
        {
            using var ms = new MemoryStream();
            WebSocketReceiveResult res;
            do
            {
                res = await ws.ReceiveAsync(seg, ct);
                if (res.MessageType == WebSocketMessageType.Close)
                {
                    try { await ws.CloseOutputAsync(WebSocketCloseStatus.NormalClosure, null, ct); } catch { }
                    return;
                }
                ms.Write(seg.Array!, 0, res.Count);
            } while (!res.EndOfMessage);

            if (res.MessageType == WebSocketMessageType.Text)
                OnText?.Invoke(Encoding.UTF8.GetString(ms.GetBuffer(), 0, (int)ms.Length));
            else if (res.MessageType == WebSocketMessageType.Binary)
                OnBinary?.Invoke(ms.ToArray());
        }
    }

    async Task SendRaw(ClientWebSocket ws, byte[] data, WebSocketMessageType type)
    {
        // ClientWebSocket forbids concurrent sends; serialize them.
        await _sendLock.WaitAsync();
        try
        {
            if (ws.State == WebSocketState.Open)
                await ws.SendAsync(data, type, endOfMessage: true, CancellationToken.None);
        }
        catch { }
        finally { _sendLock.Release(); }
    }

    static bool IsLoopback(string host)
        => host is "127.0.0.1" or "::1" or "localhost";

    static (Uri url, string? token) StripToken(Uri u)
    {
        string? token = null;
        var kept = new List<string>();
        foreach (var pair in u.Query.TrimStart('?').Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var eq = pair.IndexOf('=');
            var key = eq >= 0 ? pair[..eq] : pair;
            if (key == "token") token = Uri.UnescapeDataString(eq >= 0 ? pair[(eq + 1)..] : "");
            else kept.Add(pair);
        }
        // Advertise our version so the relay can show it in the dashboard /
        // list_devices. Unlike the token it rides in the URL — the server reads
        // ?version= off the dial.
        if (!kept.Any(p => p.StartsWith("version=", StringComparison.Ordinal)))
            kept.Add("version=" + Uri.EscapeDataString(AppVersion.Current));
        var b = new UriBuilder(u) { Query = string.Join('&', kept) };
        return (b.Uri, token);
    }
}

// AppVersion is the one monorepo version, taken from the assembly's informational
// version — which the .csproj sets from the repo-root VERSION file at build time.
// Any build-metadata suffix the SDK appends (e.g. "0.4.0+<gitsha>") is trimmed.
static class AppVersion
{
    public static string Current { get; } =
        (Assembly.GetExecutingAssembly()
            .GetCustomAttribute<AssemblyInformationalVersionAttribute>()
            ?.InformationalVersion ?? "dev")
        .Split('+')[0];
}
