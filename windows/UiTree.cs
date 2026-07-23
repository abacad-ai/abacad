using System.Diagnostics;
using System.Runtime.InteropServices;
using FlaUI.Core.AutomationElements;

namespace Abacad;

// Walks the UI Automation tree of the foreground window and emits the same flat
// shape the Android/macOS clients produce, so the server's UITree decoding and the
// agent's reasoning are identical across platforms:
//   { "pkg": <process name>, "nodes": [ {cls, text, id, clickable, bounds:[l,t,r,b]} ] }
//
// Bounds are UIA BoundingRectangle in physical screen pixels — the same space
// InputInjection clicks in — so a node's bounds map directly to a click point.
//
// Uses the FlaUI/UIA3 client (see Uia.cs). This is the exact same UI-Automation
// capability the old System.Windows.Automation version had; only the binding
// changed when the app moved to WinUI 3.
static class UiTree
{
    const int MaxNodes = 3000; // matches the Android/macOS BFS cap

    public static Dictionary<string, object?>? Capture()
    {
        IntPtr hwnd = GetForegroundWindow();
        if (hwnd == IntPtr.Zero) return null;

        AutomationElement? root;
        try { root = Uia.Automation.FromHandle(hwnd); }
        catch { return null; }
        if (root is null) return null;

        var nodes = new List<Dictionary<string, object?>>();
        var queue = new Queue<AutomationElement>();
        queue.Enqueue(root);
        int enqueued = 1;

        while (queue.Count > 0 && nodes.Count < MaxNodes)
        {
            var el = queue.Dequeue();
            if (Describe(el) is { } node) nodes.Add(node);
            AutomationElement[] children;
            try { children = el.FindAllChildren(); }
            catch { continue; }
            foreach (var child in children)
            {
                if (enqueued >= MaxNodes) break;
                queue.Enqueue(child);
                enqueued++;
            }
        }

        return new Dictionary<string, object?> { ["pkg"] = ProcessName(hwnd), ["nodes"] = nodes };
    }

    static Dictionary<string, object?>? Describe(AutomationElement el)
    {
        try
        {
            // FlaUI's ControlType enum already reads as a short role name ("Button").
            string cls = el.Properties.ControlType.ValueOrDefault.ToString();

            // Prefer visible text: editable value, then accessible name, then help.
            string text = "";
            var valuePattern = el.Patterns.Value.PatternOrDefault;
            if (valuePattern != null) text = valuePattern.Value.ValueOrDefault ?? "";
            if (string.IsNullOrEmpty(text)) text = el.Properties.Name.ValueOrDefault ?? "";
            if (string.IsNullOrEmpty(text)) text = el.Properties.HelpText.ValueOrDefault ?? "";

            string id = el.Properties.AutomationId.ValueOrDefault ?? "";
            bool clickable = el.Patterns.Invoke.IsSupported
                          || el.Patterns.Toggle.IsSupported
                          || el.Patterns.SelectionItem.IsSupported;

            var r = el.Properties.BoundingRectangle.ValueOrDefault;
            bool hasFrame = !r.IsEmpty && r.Width > 0 && r.Height > 0;
            int[] bounds = hasFrame
                ? new[] { r.Left, r.Top, r.Right, r.Bottom }
                : new[] { 0, 0, 0, 0 };

            // Skip nodes with neither a role, text, nor a frame (e.g. the bare root).
            if (cls.Length == 0 && text.Length == 0 && !hasFrame) return null;

            return new Dictionary<string, object?>
            {
                ["cls"] = cls,
                ["text"] = text,
                ["id"] = id,
                ["clickable"] = clickable,
                ["bounds"] = bounds,
            };
        }
        catch { return null; } // stale/inaccessible element — drop it
    }

    static string ProcessName(IntPtr hwnd)
    {
        GetWindowThreadProcessId(hwnd, out uint pid);
        try { using var p = Process.GetProcessById((int)pid); return p.ProcessName; }
        catch { return ""; }
    }

    [DllImport("user32.dll")]
    static extern IntPtr GetForegroundWindow();

    [DllImport("user32.dll")]
    static extern uint GetWindowThreadProcessId(IntPtr hWnd, out uint lpdwProcessId);
}
