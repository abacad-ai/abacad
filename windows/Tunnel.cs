using System.Net.Sockets;
using System.Text;

namespace Abacad;

// The binary tunnel lane, mirroring macos/Tunnel.swift and internal/protocol/
// stream.go. The server opens a stream (Open with a "host:port"); this dials that
// TCP target and pipes bytes both ways. Frames:
//   [type:1][stream id:8 BE][payload]   type 1=Open 2=Data 3=Close
// Streams are only ever opened from the server side; the device just answers.
sealed class Tunnel
{
    const byte OPEN = 1;
    const byte DATA = 2;
    const byte CLOSE = 3;

    /// Sends a binary frame back over the WebSocket. Set by the owner.
    public Action<byte[]>? SendFrame;

    readonly object _gate = new();
    readonly HashSet<ulong> _known = new();               // ids we've accepted (so Close fires once)
    readonly Dictionary<ulong, TcpClient> _conns = new();
    readonly Dictionary<ulong, NetworkStream> _streams = new();
    bool _closedAll;

    /// Handle one inbound binary frame from the server.
    public void Handle(byte[] frame)
    {
        if (frame.Length < 9) return;
        byte type = frame[0];
        ulong id = 0;
        for (int i = 0; i < 8; i++) id = (id << 8) | frame[1 + i];
        var payload = frame[9..];
        switch (type)
        {
            case OPEN: Open(id, Encoding.UTF8.GetString(payload)); break;
            case DATA: Write(id, payload); break;
            case CLOSE: Close(id); break;
        }
    }

    void Open(ulong id, string target)
    {
        int colon = target.LastIndexOf(':');
        if (colon < 0 || !int.TryParse(target[(colon + 1)..], out var port))
        {
            EmitClose(id, $"bad target {target}");
            return;
        }
        var host = target[..colon];
        // Refuse targets with no legitimate tunnel use and clear SSRF value: the
        // cloud metadata endpoint (169.254.169.254) and other link-local /
        // unspecified / multicast addresses. Loopback and private ranges stay
        // allowed — reaching this PC's own services and LAN is the point. The
        // server enforces the same policy; this is device-side defense in depth.
        if (IsBlockedTargetHost(host))
        {
            EmitClose(id, $"target {host} is not an allowed address");
            return;
        }

        lock (_gate) { if (_closedAll) return; _known.Add(id); }

        _ = Task.Run(async () =>
        {
            var client = new TcpClient();
            try { await client.ConnectAsync(host, port); }
            catch (Exception e) { client.Dispose(); EmitClose(id, e.Message); return; }

            NetworkStream ns;
            lock (_gate)
            {
                if (_closedAll || !_known.Contains(id)) { client.Dispose(); return; }
                _conns[id] = client;
                ns = client.GetStream();
                _streams[id] = ns;
            }
            await Receive(id, ns);
        });
    }

    async Task Receive(ulong id, NetworkStream ns)
    {
        var buf = new byte[64 * 1024];
        try
        {
            while (true)
            {
                int n = await ns.ReadAsync(buf);
                if (n <= 0) { EmitClose(id, ""); return; } // EOF
                SendFrame?.Invoke(Frame(DATA, id, buf[..n]));
            }
        }
        catch (Exception e) { EmitClose(id, e.Message); }
    }

    void Write(ulong id, byte[] data)
    {
        NetworkStream? ns;
        lock (_gate) _streams.TryGetValue(id, out ns);
        if (ns is null) return;
        _ = ns.WriteAsync(data).AsTask().ContinueWith(
            t => { if (t.IsFaulted) EmitClose(id, "write failed"); },
            TaskContinuationOptions.OnlyOnFaulted);
    }

    // Server told us the stream closed: tear down locally, don't echo a Close.
    void Close(ulong id)
    {
        TcpClient? c = null;
        lock (_gate) { _known.Remove(id); if (_conns.Remove(id, out var v)) c = v; _streams.Remove(id); }
        c?.Close();
    }

    // Tear down locally and tell the server the stream closed (empty reason = EOF).
    // Only emits for ids we actually accepted, so a rejected Open stays silent and
    // a stream closes exactly once (mirrors the Swift client's removeValue guard).
    void EmitClose(ulong id, string reason)
    {
        bool emit;
        TcpClient? c = null;
        lock (_gate) { emit = _known.Remove(id); if (_conns.Remove(id, out var v)) c = v; _streams.Remove(id); }
        c?.Close();
        if (emit) SendFrame?.Invoke(Frame(CLOSE, id, Encoding.UTF8.GetBytes(reason)));
    }

    public void CloseAll()
    {
        List<TcpClient> all;
        lock (_gate)
        {
            _closedAll = true;
            all = _conns.Values.ToList();
            _conns.Clear();
            _streams.Clear();
            _known.Clear();
        }
        foreach (var c in all) c.Close();
    }

    /// Best-effort SSRF guard: block link-local (incl. 169.254.169.254 metadata),
    /// unspecified, and multicast literals. Numeric range checks apply only to real
    /// IPv4 literals so a hostname like "224.example.com" isn't flagged. Loopback
    /// and private ranges are intentionally allowed.
    public static bool IsBlockedTargetHost(string host)
    {
        var h = host.ToLowerInvariant();
        // IPv6: unspecified, link-local (fe80::/10), multicast (ff00::/8).
        if (h == "::" || h.StartsWith("fe80:") || (h.StartsWith("ff") && h.Contains(':')))
            return true;
        // IPv4: only judge genuine dotted-quad literals.
        var parts = h.Split('.');
        if (parts.Length == 4)
        {
            var octets = new int[4];
            for (int i = 0; i < 4; i++)
            {
                if (!int.TryParse(parts[i], out var n) || n < 0 || n > 255) return false;
                octets[i] = n;
            }
            if (octets is [0, 0, 0, 0]) return true;              // unspecified
            if (octets[0] == 169 && octets[1] == 254) return true; // link-local incl. metadata
            if (octets[0] >= 224 && octets[0] <= 239) return true; // multicast
        }
        return false;
    }

    static byte[] Frame(byte type, ulong id, byte[] payload)
    {
        var outb = new byte[9 + payload.Length];
        outb[0] = type;
        for (int i = 0; i < 8; i++) outb[1 + i] = (byte)(id >> (8 * (7 - i))); // big-endian
        Array.Copy(payload, 0, outb, 9, payload.Length);
        return outb;
    }
}
