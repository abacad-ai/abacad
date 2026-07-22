using System.Diagnostics;
using System.Runtime.InteropServices;

namespace Abacad;

// screen_recording file channel on Windows: drives ffmpeg's gdigrab to record the
// desktop to a temp .mp4 (H.264), then uploads it to /blobs on stop — the moving-
// picture counterpart of screenshot.
//
// One recording at a time. The transfer is async: Stop() flips to "uploading" and
// hands ffmpeg-finalize + upload to a background task, so a big clip never blocks
// the command window; the agent polls Status() until the blob id appears. The temp
// file is deleted after a successful upload (automatic retention). Video only (RFB
// carries no audio, so audio would be asymmetric across channels).
sealed class ScreenRecorder
{
    public static readonly ScreenRecorder Shared = new();

    readonly object _gate = new();
    string _phase = "idle"; // idle | recording | uploading | ready | failed
    Process? _proc;
    string _path = "";
    DateTime _startAt;
    int _width, _height, _fps;
    long _durationMs, _sizeBytes;
    string _blobId = "", _sha256 = "", _error = "";

    [DllImport("user32.dll")] static extern int GetSystemMetrics(int nIndex);
    const int SM_CXSCREEN = 0, SM_CYSCREEN = 1;

    /// Dispatch a screen_recording action. blobs is the caller-resolved data-plane
    /// client (the point of the file channel is to transfer the clip afterward).
    public Dictionary<string, object?> Handle(Dictionary<string, object?> p, BlobClient blobs)
    {
        switch (p.Str("action"))
        {
            case "start":
                var file = p.TryGetValue("file", out var fv) && fv is Dictionary<string, object?> fd
                    ? fd : new Dictionary<string, object?>();
                return Start(file, blobs);
            case "stop":
                return Stop(blobs);
            case "status":
                lock (_gate) return Status();
            default:
                throw new CmdException("screen_recording action must be \"start\", \"stop\", or \"status\"");
        }
    }

    Dictionary<string, object?> Start(IReadOnlyDictionary<string, object?> file, BlobClient blobs)
    {
        lock (_gate)
        {
            if (_phase == "recording")
                throw new CmdException("a recording is already in progress; stop it first");

            int w = GetSystemMetrics(SM_CXSCREEN) & ~1; // even dims for yuv420p / H.264
            int h = GetSystemMetrics(SM_CYSCREEN) & ~1;
            int fps = file.Int("fps", 0);
            if (fps <= 0) fps = 30;
            string path = Path.Combine(Path.GetTempPath(), $"abacad-rec-{DateTime.UtcNow.Ticks}.mp4");

            var psi = new ProcessStartInfo
            {
                FileName = "ffmpeg",
                UseShellExecute = false,
                CreateNoWindow = true,
                RedirectStandardInput = true,
                RedirectStandardError = true,
            };
            foreach (var a in new[]
            {
                "-loglevel", "error", "-y",
                "-f", "gdigrab", "-framerate", fps.ToString(), "-i", "desktop",
                "-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p",
                "-movflags", "+faststart", path,
            })
                psi.ArgumentList.Add(a);

            Process proc;
            try
            {
                proc = Process.Start(psi) ?? throw new CmdException("could not start ffmpeg");
            }
            catch (System.ComponentModel.Win32Exception)
            {
                throw new CmdException("ffmpeg not found on PATH — install ffmpeg to record the screen");
            }

            _phase = "recording";
            _proc = proc;
            _path = path;
            _startAt = DateTime.UtcNow;
            _width = w; _height = h; _fps = fps;
            _durationMs = 0; _sizeBytes = 0;
            _blobId = ""; _sha256 = ""; _error = "";

            // Best-effort safety cap: auto-stop after max_duration_seconds.
            int cap = file.Int("max_duration_seconds", 0);
            if (cap > 0)
                _ = Task.Run(async () => { await Task.Delay(cap * 1000); Stop(blobs); });

            return new Dictionary<string, object?>
            { ["state"] = "recording", ["width"] = w, ["height"] = h, ["fps"] = fps };
        }
    }

    Dictionary<string, object?> Stop(BlobClient blobs)
    {
        Process proc;
        string path;
        lock (_gate)
        {
            if (_phase != "recording" || _proc == null) return Status();
            proc = _proc;
            path = _path;
            _proc = null;
            _durationMs = (long)(DateTime.UtcNow - _startAt).TotalMilliseconds;
            _phase = "uploading";
        }

        _ = Task.Run(async () =>
        {
            // Ask ffmpeg to quit cleanly ("q") so it writes the moov atom, drain
            // stderr, and wait for exit.
            string err = "";
            try
            {
                await proc.StandardInput.WriteLineAsync("q");
                proc.StandardInput.Close();
                err = await proc.StandardError.ReadToEndAsync();
                proc.WaitForExit();
            }
            catch { /* fall through to size check */ }

            long size = FileSize(path);
            if (size == 0)
            {
                err = err.Trim();
                lock (_gate)
                {
                    _phase = "failed";
                    _error = "recording produced no data" + (err.Length > 0 ? $" ({Truncate(err)})" : "");
                    _sizeBytes = 0;
                }
                TryDelete(path);
                return;
            }
            try
            {
                var (id, _, sha) = await blobs.Upload(path);
                lock (_gate) { _phase = "ready"; _blobId = id; _sha256 = sha; _sizeBytes = size; }
                TryDelete(path); // auto-retention: keep only the store copy
            }
            catch (Exception e)
            {
                lock (_gate) { _phase = "failed"; _error = e.Message; _sizeBytes = size; }
            }
        });

        lock (_gate) return Status();
    }

    // Renders the current state; the caller holds _gate.
    Dictionary<string, object?> Status()
    {
        var o = new Dictionary<string, object?> { ["state"] = _phase };
        if (_width > 0) o["width"] = _width;
        if (_height > 0) o["height"] = _height;
        if (_fps > 0) o["fps"] = _fps;
        switch (_phase)
        {
            case "recording":
                o["elapsed_ms"] = (long)(DateTime.UtcNow - _startAt).TotalMilliseconds;
                o["size_bytes"] = FileSize(_path);
                break;
            case "uploading":
            case "ready":
            case "failed":
                o["duration_ms"] = _durationMs;
                o["size_bytes"] = _sizeBytes;
                o["codec"] = "h264";
                o["transfer_state"] = _phase == "ready" ? "ready" : _phase == "failed" ? "failed" : "uploading";
                if (_blobId.Length > 0) o["blob_id"] = _blobId;
                if (_sha256.Length > 0) o["sha256"] = _sha256;
                if (_error.Length > 0) o["error"] = _error;
                break;
        }
        return o;
    }

    static long FileSize(string path)
    {
        try { return new FileInfo(path).Length; } catch { return 0; }
    }

    static void TryDelete(string path)
    {
        try { File.Delete(path); } catch { /* best effort */ }
    }

    static string Truncate(string s) => s.Length > 200 ? s[..200] : s;
}
