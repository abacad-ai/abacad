using System.Net.Http;
using System.Runtime.InteropServices;
using System.Text;

namespace Abacad;

// `abacad connect` — the device-authorization grant (RFC 8628) that enrolls this
// machine without pasting a token into the tray settings. It asks the server for
// a short code, prints the URL to approve it, polls until the human approves in
// their browser, then stores the issued wss://…?token=… URL via Prefs (DPAPI) so
// the tray app auto-connects on next launch. The console counterpart of the
// SettingsForm paste flow, and the peer of the Linux client's `abacad connect`.
static class ConnectFlow
{
    const string DefaultServer = "https://abacad.ai";

    public static int Run(string[] args)
    {
        EnsureConsole();

        var server = DefaultServer;
        for (var i = 1; i < args.Length; i++)
        {
            if ((args[i] == "--server" || args[i] == "-s") && i + 1 < args.Length)
                server = args[++i];
            else if (args[i].StartsWith("--server=", StringComparison.Ordinal))
                server = args[i]["--server=".Length..];
        }
        server = server.Trim().TrimEnd('/');
        if (server.Length == 0)
        {
            Console.Error.WriteLine("empty --server");
            return 1;
        }

        try
        {
            return RunAsync(server).GetAwaiter().GetResult();
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine($"connect failed: {ex.Message}");
            return 1;
        }
    }

    static async Task<int> RunAsync(string server)
    {
        using var http = new HttpClient { Timeout = TimeSpan.FromSeconds(30) };

        // 1. Start the pairing, reporting our platform so the approval page shows it.
        var startReq = Body(new Dictionary<string, object?> { ["platform"] = "windows" });
        var startResp = await http.PostAsync(server + "/api/devices/pair/start", startReq);
        var startText = await startResp.Content.ReadAsStringAsync();
        if (startResp.StatusCode is not (System.Net.HttpStatusCode.Created or System.Net.HttpStatusCode.OK))
        {
            Console.Error.WriteLine("start pairing: " + ServerError(startText, startResp));
            return 1;
        }
        var start = Json.Object(startText);
        if (start is null || start.Str("device_code").Length == 0)
        {
            Console.Error.WriteLine("start pairing: unexpected response");
            return 1;
        }
        var deviceCode = start.Str("device_code");
        var userCode = start.Str("user_code");
        var link = start.Str("verification_uri_complete");
        if (link.Length == 0) link = start.Str("verification_uri");
        var interval = Math.Max(start.Int("interval"), 1);
        var expiresIn = Math.Max(start.Int("expires_in"), 60);

        Console.WriteLine();
        Console.WriteLine("To connect this device, open:");
        Console.WriteLine();
        Console.WriteLine("    " + link);
        Console.WriteLine();
        Console.WriteLine($"and approve it while signed in (code: {userCode}). Waiting...");

        // 2. Poll until the human approves, honoring the interval + lifetime hints.
        var deadline = DateTime.UtcNow.AddSeconds(expiresIn);
        while (true)
        {
            if (DateTime.UtcNow > deadline)
            {
                Console.Error.WriteLine("timed out waiting for approval");
                return 1;
            }
            var pollReq = Body(new Dictionary<string, object?> { ["device_code"] = deviceCode });
            var resp = await http.PostAsync(server + "/api/devices/pair/poll", pollReq);
            var text = await resp.Content.ReadAsStringAsync();

            switch ((int)resp.StatusCode)
            {
                case 200:
                    var poll = Json.Object(text);
                    var wss = poll?.Str("wss_url") ?? "";
                    var token = poll?.Str("device_token") ?? "";
                    if (wss.Length == 0)
                    {
                        Console.Error.WriteLine("approved but server returned no wss_url");
                        return 1;
                    }
                    // The server's wss_url already carries ?token=; keep it whole (the
                    // format Prefs/WebSocketClient expect), re-attaching only if absent.
                    Prefs.ServerUrl = WithToken(wss, token);
                    Console.WriteLine();
                    Console.WriteLine("Approved. Credentials saved.");
                    Console.WriteLine("Start abacad (the tray app) to go online — it auto-connects.");
                    return 0;
                case 202: // still pending
                    await Task.Delay(TimeSpan.FromSeconds(interval));
                    break;
                default: // 403 denied / 404 unknown / 410 expired-or-used → terminal
                    Console.Error.WriteLine(ServerError(text, resp));
                    return 1;
            }
        }
    }

    static StringContent Body(Dictionary<string, object?> obj) =>
        new(Json.String(obj), Encoding.UTF8, "application/json");

    // WithToken keeps the token in the URL (Prefs stores one combined string that
    // WebSocketClient splits at dial time). The server already appends ?token=, so
    // this only adds it in the unlikely case it's missing.
    static string WithToken(string wss, string token)
    {
        if (token.Length == 0 || wss.Contains("token=", StringComparison.Ordinal)) return wss;
        var sep = wss.Contains('?') ? "&" : "?";
        return wss + sep + "token=" + Uri.EscapeDataString(token);
    }

    static string ServerError(string text, HttpResponseMessage resp)
    {
        var msg = Json.Object(text)?.Str("error") ?? "";
        return msg.Length > 0 ? msg : $"server said {(int)resp.StatusCode}";
    }

    // A WinExe has no console, so `abacad connect` from a terminal can't print
    // until we attach to the launching console (or allocate one if launched with
    // none, e.g. from Explorer). Matches the P/Invoke style used across the client.
    static void EnsureConsole()
    {
        if (!AttachConsole(AttachParentProcess))
            AllocConsole();
    }

    const uint AttachParentProcess = 0xFFFFFFFF;

    [DllImport("kernel32.dll", SetLastError = true)]
    static extern bool AttachConsole(uint dwProcessId);

    [DllImport("kernel32.dll", SetLastError = true)]
    static extern bool AllocConsole();
}
