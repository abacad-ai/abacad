using System.Net.Http;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text.Json;

namespace Abacad;

// Device side of the /blobs data plane, backing the push_file / pull_file verbs.
// File bytes ride HTTP — not the command WebSocket — so a large file never has to
// be base64'd onto a text frame. Authenticated with the same per-device token the
// socket uses, carried in the Authorization header. Streamed end to end: download
// copies the response straight to a temp file (then moves it), upload streams from
// the file handle. Mirrors the macOS/Linux/Android blob clients.
sealed class BlobClient
{
    readonly string _base;      // e.g. https://host/blobs
    readonly string? _token;

    // No timeout: a multi-GB transfer may legitimately run long. These are
    // one-shot calls, unlike the long-lived socket.
    static readonly HttpClient Http = new() { Timeout = Timeout.InfiniteTimeSpan };

    BlobClient(string baseUrl, string? token) { _base = baseUrl; _token = token; }

    /// Derive the /blobs endpoint from the relay URL: same host, over http(s)
    /// instead of ws(s). Returns null if the URL isn't a ws/wss URL, which
    /// disables file transfer rather than guessing.
    public static BlobClient? FromServerUrl(string raw)
    {
        if (!Uri.TryCreate(raw.Trim(), UriKind.Absolute, out var u)) return null;
        var scheme = u.Scheme.ToLowerInvariant() switch { "wss" => "https", "ws" => "http", _ => (string?)null };
        if (scheme is null) return null;
        string? token = null;
        foreach (var pair in u.Query.TrimStart('?').Split('&', StringSplitOptions.RemoveEmptyEntries))
        {
            var eq = pair.IndexOf('=');
            if ((eq >= 0 ? pair[..eq] : pair) == "token")
                token = Uri.UnescapeDataString(eq >= 0 ? pair[(eq + 1)..] : "");
        }
        var b = new UriBuilder(u) { Scheme = scheme, Path = "/blobs", Query = "", Fragment = "" };
        return new BlobClient(b.Uri.GetLeftPart(UriPartial.Path), token);
    }

    void Auth(HttpRequestMessage req)
    {
        if (_token is not null) req.Headers.Authorization = new AuthenticationHeaderValue("Bearer", _token);
    }

    /// Stream the blob to destPath and return (bytesWritten, hexSha256). Downloads
    /// to a temp file in the destination directory and moves it into place, so a
    /// reader never sees a partial file. The parent directory must already exist.
    public async Task<(long size, string sha256)> Download(string blobId, string destPath, int mode)
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, $"{_base}/{blobId}");
        Auth(req);
        using var resp = await Http.SendAsync(req, HttpCompletionOption.ResponseHeadersRead);
        if (!resp.IsSuccessStatusCode)
            throw new CmdException($"blob download failed: HTTP {(int)resp.StatusCode}{await Snippet(resp)}");

        var full = Path.GetFullPath(destPath);
        var dir = Path.GetDirectoryName(full) ?? throw new CmdException($"no parent directory for {destPath}");
        var tmp = Path.Combine(dir, ".abacad-dl-" + Guid.NewGuid().ToString("N"));
        long size;
        string sha;
        try
        {
            await using (var body = await resp.Content.ReadAsStreamAsync())
            await using (var file = File.Create(tmp))
            using (var sha256 = SHA256.Create())
            await using (var crypto = new CryptoStream(file, sha256, CryptoStreamMode.Write))
            {
                await body.CopyToAsync(crypto);
                crypto.FlushFinalBlock();
                sha = Convert.ToHexString(sha256.Hash!).ToLowerInvariant();
            }
            if (File.Exists(full)) File.Delete(full);
            File.Move(tmp, full);
            size = new FileInfo(full).Length; // authoritative, after all buffers flushed
        }
        catch
        {
            try { File.Delete(tmp); } catch { /* best effort */ }
            throw;
        }
        // Windows has no POSIX permission bits; the closest honest mapping is the
        // read-only attribute when the owner-write bit (0200) is absent.
        if ((mode & 0x80) == 0)
        {
            try { File.SetAttributes(full, File.GetAttributes(full) | FileAttributes.ReadOnly); } catch { }
        }
        return (size, sha);
    }

    /// Stream srcPath to /blobs and return (blobId, size, hexSha256).
    public async Task<(string id, long size, string sha256)> Upload(string srcPath)
    {
        if (Directory.Exists(srcPath)) throw new CmdException($"{srcPath} is a directory, not a file");
        if (!File.Exists(srcPath)) throw new CmdException($"no such file: {srcPath}");

        using var req = new HttpRequestMessage(HttpMethod.Post, _base);
        Auth(req);
        await using var fs = File.OpenRead(srcPath);
        req.Content = new StreamContent(fs);
        req.Content.Headers.ContentType = new MediaTypeHeaderValue("application/octet-stream");

        using var resp = await Http.SendAsync(req, HttpCompletionOption.ResponseHeadersRead);
        if ((int)resp.StatusCode != 201)
            throw new CmdException($"blob upload failed: HTTP {(int)resp.StatusCode}{await Snippet(resp)}");

        using var doc = JsonDocument.Parse(await resp.Content.ReadAsStringAsync());
        var root = doc.RootElement;
        return (
            root.GetProperty("id").GetString() ?? "",
            root.GetProperty("size").GetInt64(),
            root.TryGetProperty("sha256", out var s) ? s.GetString() ?? "" : "");
    }

    static async Task<string> Snippet(HttpResponseMessage resp)
    {
        try
        {
            var s = (await resp.Content.ReadAsStringAsync()).Trim();
            if (s.Length == 0) return "";
            return " — " + (s.Length > 200 ? s[..200] : s);
        }
        catch { return ""; }
    }
}
