using System.Net;
using System.Net.Http.Headers;
using System.Text;

namespace Abacad;

/// <summary>
/// Device-first self-enrollment: the PC announces itself to a relay, receives its
/// permanent id and token, and shows that id plus a rotating claim code until a
/// human binds it to an account at &lt;relay&gt;/claim.
///
/// A deliberate 1:1 transliteration of the Linux reference implementation
/// (linux/internal/enroll) and its macOS peer (Enrollment.swift) — same states,
/// same error taxonomy, same interval clamping, no novel logic. This file cannot
/// be compiled anywhere in the current environment, so "obviously the same as the
/// two verified ports" is the only assurance available; keep it that way.
///
/// Wire contract:
///   POST /api/devices/register        -> {device_id, device_token, claim_code, …}
///   POST /api/devices/self/heartbeat  -> {claimed, claim_code, …} | {claimed, claimed_by}
///
/// The token survives the claim unchanged, so unclaimed -> claimed needs no
/// re-key: the same credential that polls the heartbeat later dials /device.
/// </summary>
static class Enrollment
{
    public const string DefaultRelay = "https://abacad.ai";

    /// <summary>The relay does not recognize our token: wipe it and re-register.</summary>
    public sealed class UnknownTokenException : Exception { }

    /// <summary>The relay predates self-enrollment: fall back to a connection URL.</summary>
    public sealed class NotSupportedException : Exception { }

    public sealed class TransportException : Exception
    {
        public TransportException(string message) : base(message) { }
    }

    public sealed record Registration(
        string DeviceId, string DeviceToken, string ClaimCode, int ClaimExpiresIn, int HeartbeatIn);

    public sealed record Status(
        bool Claimed, string DeviceId, string ClaimCode, int ClaimExpiresIn, int HeartbeatIn,
        string Name,
        // The account that claimed this device. Shown in the window so a claim by
        // someone who merely read the screen isn't silent to the person here.
        string ClaimedBy);

    public static async Task<Registration> RegisterAsync(
        HttpClient http, string relay, string platform, string name, string version)
    {
        var body = JsonBody(new Dictionary<string, object?>
        {
            ["platform"] = platform, ["name"] = name, ["version"] = version,
        });
        var (code, text) = await PostAsync(http, Normalize(relay) + "/api/devices/register", body, null);
        if (code == (int)HttpStatusCode.NotFound || code == (int)HttpStatusCode.MethodNotAllowed)
            throw new NotSupportedException();
        if (code != (int)HttpStatusCode.Created && code != (int)HttpStatusCode.OK)
            throw new TransportException(ServerError(text, code));

        var o = Json.Object(text);
        var id = o?.Str("device_id") ?? "";
        var token = o?.Str("device_token") ?? "";
        if (id.Length == 0 || token.Length == 0)
            throw new TransportException("relay returned an incomplete registration");
        return new Registration(id, token, o!.Str("claim_code"),
            o.Int("claim_expires_in"), o.Int("heartbeat_in"));
    }

    public static async Task<Status> HeartbeatAsync(HttpClient http, string relay, string token)
    {
        // Header only: these endpoints are new, so there is no legacy ?token=
        // shape to support and no reason to put the secret in a URL.
        var (code, text) = await PostAsync(
            http, Normalize(relay) + "/api/devices/self/heartbeat", null, token);
        if (code == (int)HttpStatusCode.NotFound || code == (int)HttpStatusCode.Gone
            || code == (int)HttpStatusCode.Unauthorized)
            throw new UnknownTokenException();
        if (code != (int)HttpStatusCode.OK)
            throw new TransportException(ServerError(text, code));

        var o = Json.Object(text);
        if (o == null) throw new TransportException("relay returned an unreadable heartbeat");
        return new Status(o.Bool("claimed"), o.Str("device_id"), o.Str("claim_code"),
            o.Int("claim_expires_in"), o.Int("heartbeat_in"), o.Str("name"), o.Str("claimed_by"));
    }

    /// <summary>
    /// The WebSocket URL to dial once claimed. http(s) maps to ws(s) so a loopback
    /// dev relay stays on plain ws while a real deployment gets wss — matching
    /// WebSocketClient's own refusal to speak ws:// off-loopback.
    /// </summary>
    public static string DeviceUrl(string relay)
    {
        var b = Normalize(relay);
        if (b.StartsWith("https://", StringComparison.OrdinalIgnoreCase))
            b = "wss://" + b["https://".Length..];
        else if (b.StartsWith("http://", StringComparison.OrdinalIgnoreCase))
            b = "ws://" + b["http://".Length..];
        return b + "/device";
    }

    /// <summary>Clamp a server-advertised poll hint so a bad value can't spin us.</summary>
    public static TimeSpan Interval(int seconds) =>
        TimeSpan.FromSeconds(seconds < 5 ? 20 : seconds > 300 ? 300 : seconds);

    public static string Normalize(string relay)
    {
        var s = (relay ?? "").Trim().TrimEnd('/');
        return s.Length == 0 ? DefaultRelay : s;
    }

    /// <summary>A recognizable default name, better than "New device".</summary>
    public static string DefaultDeviceName()
    {
        try
        {
            var n = Environment.MachineName;
            return string.IsNullOrWhiteSpace(n) ? "Windows PC" : n;
        }
        catch { return "Windows PC"; }
    }

    static StringContent JsonBody(Dictionary<string, object?> obj) =>
        new(Json.String(obj), Encoding.UTF8, "application/json");

    static async Task<(int, string)> PostAsync(
        HttpClient http, string url, HttpContent? body, string? bearer)
    {
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Post, url);
            if (body != null) req.Content = body;
            else req.Content = new StringContent("", Encoding.UTF8);
            if (bearer != null)
                req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", bearer);
            using var resp = await http.SendAsync(req);
            return ((int)resp.StatusCode, await resp.Content.ReadAsStringAsync());
        }
        catch (Exception e)
        {
            throw new TransportException(e.Message);
        }
    }

    static string ServerError(string text, int code)
    {
        var msg = Json.Object(text)?.Str("error") ?? "";
        return msg.Length == 0 ? $"server said {code}" : msg;
    }
}
