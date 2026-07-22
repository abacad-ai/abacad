using System.Net.WebSockets;
using System.Text;

namespace Abacad;

// Windows live channel (screen_recording live): a minimal, view-only RFB (VNC)
// server spoken directly over the dedicated reverse-connect WebSocket — mirrors
// macos/Sources/abacad/VNCServer.swift. On "start" it dials the server's VNC
// ingress and serves RFB (banner + security None + ServerInit, then a Raw-encoded
// BGRX framebuffer per request). Input messages are parsed and dropped — view only
// for now. The pixels ride this dedicated WS, never the command socket.
//
// UNVERIFIED at runtime: the RFB byte protocol (pixel-format handling especially)
// needs a real noVNC client to shake out.
sealed class VncServer
{
    public static readonly VncServer Shared = new();

    readonly object _gate = new();
    CancellationTokenSource? _cts;

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
        var cts = new CancellationTokenSource();
        lock (_gate) _cts = cts;
        _ = Task.Run(async () =>
        {
            var ws = new ClientWebSocket();
            try
            {
                await ws.ConnectAsync(new Uri(url), cts.Token);
                await Serve(new WsStream(ws), cts.Token);
            }
            catch { /* connection closed or protocol ended */ }
            finally
            {
                try { ws.Dispose(); } catch { }
                Stop();
            }
        });
    }

    public void Stop()
    {
        CancellationTokenSource? cts;
        lock (_gate) { cts = _cts; _cts = null; }
        try { cts?.Cancel(); } catch { }
    }

    static async Task Serve(WsStream s, CancellationToken ct)
    {
        await s.Write(Encoding.ASCII.GetBytes("RFB 003.008\n"), ct); // ProtocolVersion
        await s.Read(12, ct); // client version
        await s.Write(new byte[] { 1, 1 }, ct); // 1 security type: None(1)
        await s.Read(1, ct); // client selects
        await s.Write(new byte[] { 0, 0, 0, 0 }, ct); // SecurityResult OK
        await s.Read(1, ct); // ClientInit (shared flag)

        var frame = ScreenCapture.CaptureRawBGRA();
        await s.Write(ServerInit(frame.W, frame.H), ct);

        while (!ct.IsCancellationRequested)
        {
            byte type = (await s.Read(1, ct))[0];
            switch (type)
            {
                case 0: // SetPixelFormat (view-only: ignore, keep BGRX)
                    await s.Read(19, ct);
                    break;
                case 2: // SetEncodings
                    var hdr = await s.Read(3, ct);
                    int count = hdr[1] << 8 | hdr[2];
                    if (count > 0) await s.Read(count * 4, ct);
                    break;
                case 3: // FramebufferUpdateRequest
                    await s.Read(9, ct);
                    var f = ScreenCapture.CaptureRawBGRA();
                    await s.Write(FramebufferUpdate(f), ct);
                    break;
                case 4: // KeyEvent
                    await s.Read(7, ct);
                    break;
                case 5: // PointerEvent
                    await s.Read(5, ct);
                    break;
                case 6: // ClientCutText
                    var c = await s.Read(7, ct);
                    int n = c[3] << 24 | c[4] << 16 | c[5] << 8 | c[6];
                    if (n > 0) await s.Read(n, ct);
                    break;
                default:
                    throw new IOException($"unknown RFB client message {type}");
            }
        }
    }

    static byte[] ServerInit(int w, int h)
    {
        var b = new List<byte>();
        b.AddRange(Be16(w));
        b.AddRange(Be16(h));
        // 32bpp, depth 24, little-endian, true-colour, BGRX (redShift 16/green 8/blue 0).
        b.AddRange(new byte[] { 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0 });
        var name = Encoding.ASCII.GetBytes("abacad");
        b.AddRange(Be32(name.Length));
        b.AddRange(name);
        return b.ToArray();
    }

    static byte[] FramebufferUpdate((int W, int H, byte[] Pixels) f)
    {
        var b = new List<byte>(f.Pixels.Length + 16);
        b.AddRange(new byte[] { 0, 0 });  // message type 0, padding
        b.AddRange(Be16(1));               // one rectangle
        b.AddRange(Be16(0));               // x
        b.AddRange(Be16(0));               // y
        b.AddRange(Be16(f.W));
        b.AddRange(Be16(f.H));
        b.AddRange(Be32(0));               // encoding 0 = Raw
        b.AddRange(f.Pixels);
        return b.ToArray();
    }

    static byte[] Be16(int v) => new byte[] { (byte)(v >> 8), (byte)v };
    static byte[] Be32(int v) => new byte[] { (byte)(v >> 24), (byte)(v >> 16), (byte)(v >> 8), (byte)v };
}

// WsStream turns a ClientWebSocket into a byte stream with "read exactly n"
// (accumulating across frames) and binary writes — the shape RFB needs.
sealed class WsStream
{
    readonly ClientWebSocket _ws;
    readonly List<byte> _buf = new();

    public WsStream(ClientWebSocket ws) => _ws = ws;

    public async Task<byte[]> Read(int n, CancellationToken ct)
    {
        var tmp = new byte[64 * 1024];
        while (_buf.Count < n)
        {
            var r = await _ws.ReceiveAsync(tmp, ct);
            if (r.MessageType == WebSocketMessageType.Close) throw new IOException("ws closed");
            for (int i = 0; i < r.Count; i++) _buf.Add(tmp[i]);
        }
        var outb = _buf.GetRange(0, n).ToArray();
        _buf.RemoveRange(0, n);
        return outb;
    }

    public Task Write(byte[] b, CancellationToken ct) =>
        _ws.SendAsync(b, WebSocketMessageType.Binary, true, ct);
}
