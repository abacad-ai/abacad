using System.Windows.Automation;

namespace Abacad;

// Routes a parsed {id, method, params} command to a handler and produces the
// {id, ok, result|error} reply. Correlation is purely by id; replies may be sent
// out of order (screenshots are async). Malformed frames are dropped upstream with
// no reply, matching the other clients.
//
// Windows answers a superset: the mobile verbs (mapped to desktop equivalents so
// today's tools/agent work unchanged) plus the desktop-native verbs. Anything
// unrecognized returns "unknown method: X" — which is how the server keeps one
// global tool list without per-platform filtering.
sealed class CommandDispatcher
{
    /// Backs push_file / pull_file over the /blobs data plane. Set by Agent from
    /// the server URL; null disables file transfer (the verbs then say so).
    public BlobClient? Blobs { get; set; }

    /// Execute a method and return the `result` object, or throw CmdException.
    public async Task<Dictionary<string, object?>> Execute(string method, Dictionary<string, object?> p)
    {
        // Any non-screenshot command may change the screen, so invalidate the shot
        // cache before running it — the next screenshot must never serve a frame
        // captured before this action.
        if (method != "screenshot")
            ScreenshotCache.Shared.Invalidate();

        switch (method)
        {
            case "screenshot":
                return await ScreenshotCache.Shared.Screenshot(p.Bool("include_ui_tree", true));

            // Mobile verbs, mapped onto desktop input for cross-platform compatibility.
            case "tap":
                InputInjection.Click(p.Int("x"), p.Int("y"));
                return Dispatched();
            case "long_press":
                InputInjection.LongPress(p.Int("x"), p.Int("y"), p.Int("duration_ms", 600));
                return Dispatched();
            case "swipe":
                InputInjection.Drag(p.Int("x1"), p.Int("y1"), p.Int("x2"), p.Int("y2"),
                    p.Int("duration_ms", 300));
                return Dispatched();
            case "input_text":
                return new Dictionary<string, object?> { ["set"] = SetFocusedText(p.Str("text")) };

            // Desktop-native verbs.
            case "click":
                InputInjection.Click(p.Int("x"), p.Int("y"), "left", p.Int("count", 1), p.Strings("modifiers"));
                return Dispatched();
            case "right_click":
                InputInjection.RightClick(p.Int("x"), p.Int("y"));
                return Dispatched();
            case "drag":
                InputInjection.Drag(p.Int("x1"), p.Int("y1"), p.Int("x2"), p.Int("y2"),
                    p.Int("duration_ms", 300), p.Strings("modifiers"));
                return Dispatched();
            case "scroll":
                InputInjection.Scroll(p.Int("x"), p.Int("y"), p.Int("dx"), p.Int("dy"));
                return Dispatched();
            case "press_keys":
                var keys = p.Strings("keys");
                if (keys.Count == 0) throw new CmdException("press_keys requires a non-empty keys array");
                if (!InputInjection.PressChord(keys))
                    throw new CmdException($"press_keys: no recognized key in [{string.Join(", ", keys)}]");
                return new Dictionary<string, object?> { ["pressed"] = true };
            case "composite":
                var steps = p.Objects("steps");
                if (steps.Count == 0) throw new CmdException("composite requires a non-empty steps array");
                return await Composite.Run(steps);

            // File transfer over the /blobs data plane. Filesystem I/O — no display
            // or input needed, so it runs the same whether or not anyone's at the PC.
            case "push_file":
            {
                var blobs = Blobs ?? throw new CmdException("file transfer is not configured on this device");
                string blobId = p.Str("blob_id"), dest = p.Str("dest_path");
                if (blobId.Length == 0 || dest.Length == 0)
                    throw new CmdException("push_file requires blob_id and dest_path");
                var (size, sha) = await blobs.Download(blobId, dest, p.Int("mode", 0x1A4)); // 0644
                return new Dictionary<string, object?> { ["written"] = true, ["size"] = size, ["sha256"] = sha };
            }
            case "pull_file":
            {
                var blobs = Blobs ?? throw new CmdException("file transfer is not configured on this device");
                string src = p.Str("src_path");
                if (src.Length == 0) throw new CmdException("pull_file requires src_path");
                var (id, size, sha) = await blobs.Upload(src);
                return new Dictionary<string, object?> { ["blob_id"] = id, ["size"] = size, ["sha256"] = sha };
            }

            // Screen recording (file channel): ffmpeg gdigrab -> temp .mp4 -> /blobs.
            case "screen_recording":
            {
                var blobs = Blobs ?? throw new CmdException("screen recording needs the /blobs data plane, which is not configured on this device");
                return ScreenRecorder.Shared.Handle(p, blobs);
            }

            // Live view (screen_recording live channel): start a real VNC server
            // (bundled TigerVNC) on localhost and pipe the reverse-connect WS to it.
            // The client never speaks RFB itself.
            case "vnc":
                return VncPipe.Shared.Handle(p);

            // Mobile navigation keys have no desktop analogue.
            case "back":
            case "home":
            case "recents":
                throw new CmdException($"{method} has no desktop analogue — use click / press_keys");

            default:
                throw new CmdException($"unknown method: {method}");
        }
    }

    static Dictionary<string, object?> Dispatched() => new() { ["dispatched"] = true };

    /// Replace the focused field's contents via UIA ValuePattern (matches the mac/
    /// Android input_text "set text" semantics). Falls back to typing if there is no
    /// settable value pattern.
    static bool SetFocusedText(string text)
    {
        try
        {
            var focused = AutomationElement.FocusedElement;
            if (focused != null && focused.TryGetCurrentPattern(ValuePattern.Pattern, out var vp))
            {
                var pattern = (ValuePattern)vp;
                if (!pattern.Current.IsReadOnly) { pattern.SetValue(text); return true; }
            }
        }
        catch { /* fall through to typing */ }
        InputInjection.TypeText(text);
        return true;
    }
}
