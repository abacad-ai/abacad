using System.Diagnostics;
using System.Net;
using System.Net.Sockets;
using System.Net.WebSockets;

namespace Abacad;

// Windows live channel (screen_recording live): the client never implements RFB. It
// starts a real VNC server (bundled TigerVNC) bound to localhost and pipes the
// dedicated reverse-connect WebSocket to it — same shape as the Linux client's
// x11vnc pipe. RFB (handshake, encodings, dirty-rects) is the server's job; the GPL
// server ships as a separate binary beside the MIT client (aggregation).
//
// The server command is overridable via ABACAD_VNC_SERVER ("{port}" is substituted);
// the default expects a bundled winvnc under vnc\ next to the executable. It must
// serve 127.0.0.1:{port}.
sealed class VncPipe
{
    public static readonly VncPipe Shared = new();

    readonly object _gate = new();
    CancellationTokenSource? _cts;
    Process? _server;

    public Dictionary<string, object?> Handle(Dictionary<string, object?> p)
    {
        switch (p.Str("action"))
        {
            case "start":
                Start(p.Str("url"));
                return new Dictionary<string, object?> { ["started"] = true };
            case "stop":
                Stop();
                return new Dictionary<string, object?> { ["stopped"] = true };
            default:
                throw new CmdException("vnc action must be \"start\" or \"stop\"");
        }
    }

    void Start(string url)
    {
        if (url.Length == 0) throw new CmdException("vnc start requires url");
        Stop();
        int port = FreePort();
        var server = StartVncServer(port);
        var cts = new CancellationTokenSource();
        lock (_gate) { _cts = cts; _server = server; }

        _ = Task.Run(async () =>
        {
            var ws = new ClientWebSocket();
            TcpClient? tcp = null;
            try
            {
                await ws.ConnectAsync(new Uri(url), cts.Token);
                tcp = new TcpClient();
                await ConnectWithRetry(tcp, port, cts.Token);
                await Pipe(ws, tcp.GetStream(), cts.Token);
            }
            catch { /* connection closed or ended */ }
            finally
            {
                try { ws.Dispose(); } catch { }
                try { tcp?.Dispose(); } catch { }
                Stop();
            }
        });
    }

    public void Stop()
    {
        CancellationTokenSource? cts;
        Process? srv;
        lock (_gate) { cts = _cts; srv = _server; _cts = null; _server = null; }
        try { cts?.Cancel(); } catch { }
        try { if (srv is { HasExited: false }) srv.Kill(); } catch { }
    }

    static Process StartVncServer(int port)
    {
        string exe, args;
        var cmd = Environment.GetEnvironmentVariable("ABACAD_VNC_SERVER");
        if (!string.IsNullOrEmpty(cmd))
        {
            var parts = cmd.Replace("{port}", port.ToString()).Split(' ', 2);
            exe = parts[0];
            args = parts.Length > 1 ? parts[1] : "";
        }
        else
        {
            // Bundled TigerVNC next to the app. Flags mirror x11vnc's intent
            // (localhost only); adjust to the bundled server if it differs.
            exe = Path.Combine(AppContext.BaseDirectory, "vnc", "winvnc.exe");
            args = $"-localhost -rfbport {port} -nopw";
        }
        var psi = new ProcessStartInfo { FileName = exe, Arguments = args, UseShellExecute = false, CreateNoWindow = true };
        try
        {
            return Process.Start(psi) ?? throw new CmdException("could not start VNC server");
        }
        catch (System.ComponentModel.Win32Exception)
        {
            throw new CmdException("no VNC server found — bundle TigerVNC (vnc\\winvnc.exe) or set ABACAD_VNC_SERVER");
        }
    }

    static async Task ConnectWithRetry(TcpClient tcp, int port, CancellationToken ct)
    {
        for (int i = 0; i < 50; i++)
        {
            try { await tcp.ConnectAsync(IPAddress.Loopback, port, ct); return; }
            catch { await Task.Delay(100, ct); }
        }
        throw new CmdException("VNC server did not start listening");
    }

    static async Task Pipe(ClientWebSocket ws, NetworkStream tcp, CancellationToken ct)
    {
        var tcpToWs = Task.Run(async () =>
        {
            var buf = new byte[64 * 1024];
            while (!ct.IsCancellationRequested)
            {
                int n = await tcp.ReadAsync(buf, ct);
                if (n <= 0) break;
                await ws.SendAsync(new ArraySegment<byte>(buf, 0, n), WebSocketMessageType.Binary, true, ct);
            }
        }, ct);
        var wsToTcp = Task.Run(async () =>
        {
            var buf = new byte[64 * 1024];
            while (!ct.IsCancellationRequested)
            {
                var r = await ws.ReceiveAsync(buf, ct);
                if (r.MessageType == WebSocketMessageType.Close) break;
                await tcp.WriteAsync(buf.AsMemory(0, r.Count), ct);
            }
        }, ct);
        await Task.WhenAny(tcpToWs, wsToTcp);
    }

    static int FreePort()
    {
        var l = new TcpListener(IPAddress.Loopback, 0);
        l.Start();
        int p = ((IPEndPoint)l.LocalEndpoint).Port;
        l.Stop();
        return p;
    }
}
