using System.Text.Encodings.Web;
using System.Text.Json;

namespace Abacad;

// Tiny JSON helpers. The wire uses dynamic `params` objects, so we work in
// Dictionary<string, object?> via System.Text.Json rather than typed models — it
// keeps the command envelope and per-method params handling uniform and lax
// (matching the macOS/Android clients, which silently drop malformed frames).
static class Json
{
    static readonly JsonSerializerOptions Options = new()
    {
        // Relaxed escaping keeps UI-tree text and base64 readable on the wire;
        // the output is still valid JSON that the Go server decodes unchanged.
        Encoder = JavaScriptEncoder.UnsafeRelaxedJsonEscaping,
    };

    /// Parse a text frame into a top-level object. Returns null on any error.
    public static Dictionary<string, object?>? Object(string text)
    {
        try
        {
            using var doc = JsonDocument.Parse(text);
            if (doc.RootElement.ValueKind != JsonValueKind.Object) return null;
            return Convert(doc.RootElement) as Dictionary<string, object?>;
        }
        catch { return null; }
    }

    /// Serialize an object graph to a compact JSON string. Returns "{}" if it can't.
    public static string String(object? obj)
    {
        try { return JsonSerializer.Serialize(obj, Options); }
        catch { return "{}"; }
    }

    static object? Convert(JsonElement el) => el.ValueKind switch
    {
        JsonValueKind.Object => el.EnumerateObject()
            .ToDictionary(p => p.Name, p => Convert(p.Value)),
        JsonValueKind.Array => el.EnumerateArray().Select(Convert).ToList(),
        JsonValueKind.String => el.GetString(),
        JsonValueKind.Number => el.TryGetInt64(out var l) ? (object)l : el.GetDouble(),
        JsonValueKind.True => true,
        JsonValueKind.False => false,
        _ => null,
    };
}

// Convenience typed getters over a loose params dict — the C# counterpart of the
// Swift Dictionary extensions. Numbers arrive as long or double (see Convert).
static class ParamsExt
{
    public static int Int(this IReadOnlyDictionary<string, object?> d, string k, int def = 0)
    {
        if (!d.TryGetValue(k, out var v) || v is null) return def;
        return v switch
        {
            long l => (int)l,
            int i => i,
            double db => (int)db,
            string s when int.TryParse(s, out var n) => n,
            _ => def,
        };
    }

    public static double Double(this IReadOnlyDictionary<string, object?> d, string k, double def = 0)
    {
        if (!d.TryGetValue(k, out var v) || v is null) return def;
        return v switch { double db => db, long l => l, int i => i, _ => def };
    }

    public static string Str(this IReadOnlyDictionary<string, object?> d, string k, string def = "")
        => d.TryGetValue(k, out var v) && v is string s ? s : def;

    public static bool Bool(this IReadOnlyDictionary<string, object?> d, string k, bool def = false)
        => d.TryGetValue(k, out var v) && v is bool b ? b : def;

    public static List<string> Strings(this IReadOnlyDictionary<string, object?> d, string k)
        => d.TryGetValue(k, out var v) && v is List<object?> list
            ? list.OfType<string>().ToList()
            : new();

    public static List<Dictionary<string, object?>> Objects(this IReadOnlyDictionary<string, object?> d, string k)
        => d.TryGetValue(k, out var v) && v is List<object?> list
            ? list.OfType<Dictionary<string, object?>>().ToList()
            : new();
}

/// A method handler either succeeds with a result object or throws this with a
/// message that becomes the reply's `error`.
sealed class CmdException : Exception
{
    public CmdException(string message) : base(message) { }
}
