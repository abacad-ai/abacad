namespace Abacad;

// A short-lived screenshot cache with single-flight coalescing, the Windows port
// of macos/ScreenshotCache.swift. Two optimizations, both keyed off a monotonic
// screen "generation":
//
//   • Cache — a capture is reused for up to CacheWindowMs after it was taken, so
//     the dashboard's periodic poll and an agent's screenshot that land within the
//     same second share one capture + JPEG encode instead of doing two.
//
//   • Single-flight — concurrent screenshot commands that miss the cache join one
//     in-flight capture and share its result, rather than each kicking an
//     independent GDI grab.
//
// Any drive command (tap, click, scroll, input, …) calls Invalidate(), bumping the
// generation so the very next screenshot is guaranteed fresh — never a frame that
// predates the action.
sealed class ScreenshotCache
{
    public static readonly ScreenshotCache Shared = new();

    const long CacheWindowMs = 1000; // 1s

    sealed record Entry(ScreenCapture.Shot Shot, Dictionary<string, object?>? Tree, long StampMs, int Gen);

    readonly object _gate = new();
    int _gen;
    Entry? _entry;
    Task<Entry>? _inflight;
    int _inflightGen = -1;
    bool _inflightHasTree;
    int _taskId;

    /// Invalidate the cache after a screen-changing command, so the next screenshot
    /// re-captures. Called for every non-screenshot method.
    public void Invalidate()
    {
        lock (_gate) _gen++;
    }

    /// Serve a screenshot as the wire result dict, reusing a cached or in-flight
    /// capture when possible. `includeTree` requires the served frame to carry a UI
    /// tree; a cached frame without one does not satisfy a tree request.
    public async Task<Dictionary<string, object?>> Screenshot(bool includeTree)
    {
        Task<Entry> task;
        lock (_gate)
        {
            long now = Environment.TickCount64;
            if (_entry is { } e && e.Gen == _gen && now - e.StampMs < CacheWindowMs
                && (!includeTree || e.Tree != null))
                return Response(e, includeTree);

            if (_inflight != null && _inflightGen == _gen && (!includeTree || _inflightHasTree))
            {
                task = _inflight;
            }
            else
            {
                int capGen = _gen;
                bool wantTree = includeTree;
                int myId = ++_taskId;
                task = Capture(capGen, wantTree, myId);
                _inflight = task;
                _inflightGen = capGen;
                _inflightHasTree = wantTree;
            }
        }
        var result = await task.ConfigureAwait(false);
        return Response(result, includeTree);
    }

    async Task<Entry> Capture(int capGen, bool wantTree, int myId)
    {
        try
        {
            var shot = await Task.Run(() => ScreenCapture.Capture()).ConfigureAwait(false);
            // Capture the tree alongside the pixels so both describe the same screen
            // state.
            var tree = wantTree ? await Task.Run(() => UiTree.Capture()).ConfigureAwait(false) : null;
            var e = new Entry(shot, tree, Environment.TickCount64, capGen);
            lock (_gate)
            {
                if (_taskId == myId) _inflight = null;
                // Only cache if no drive command bumped the generation mid-capture;
                // otherwise the frame predates that action and must not be reused.
                if (e.Gen == _gen) _entry = e;
            }
            return e;
        }
        catch
        {
            lock (_gate) { if (_taskId == myId) _inflight = null; }
            throw;
        }
    }

    // Field stays `png_base64` for wire compatibility even though the bytes are JPEG.
    static Dictionary<string, object?> Response(Entry e, bool includeTree)
    {
        var result = new Dictionary<string, object?>
        {
            ["w"] = e.Shot.W,
            ["h"] = e.Shot.H,
            ["png_base64"] = e.Shot.Base64,
        };
        if (includeTree && e.Tree != null) result["tree"] = e.Tree;
        return result;
    }
}
