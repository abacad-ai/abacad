using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Windows.Automation;

namespace Abacad;

// Walks the UI Automation tree of the foreground window and emits the same flat
// shape the Android/macOS clients produce, so the server's UITree decoding and the
// agent's reasoning are identical across platforms:
//   { "pkg": <process name>, "nodes": [ {cls, text, id, clickable, bounds:[l,t,r,b]} ] }
//
// Bounds are UIA BoundingRectangle in physical screen pixels — the same space
// InputInjection clicks in — so a node's bounds map directly to a click point.
static class UiTree
{
    const int MaxNodes = 3000; // matches the Android/macOS BFS cap

    public static Dictionary<string, object?>? Capture()
    {
        IntPtr hwnd = GetForegroundWindow();
        if (hwnd == IntPtr.Zero) return null;

        AutomationElement? root;
        try { root = AutomationElement.FromHandle(hwnd); }
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
            AutomationElementCollection children;
            try { children = el.FindAll(TreeScope.Children, Condition.TrueCondition); }
            catch { continue; }
            foreach (AutomationElement child in children)
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
            var info = el.Current;
            string cls = TrimControlType(info.ControlType?.ProgrammaticName);
            // Prefer visible text: editable value, then accessible name, then help.
            string text = "";
            if (el.TryGetCurrentPattern(ValuePattern.Pattern, out var vp))
                text = ((ValuePattern)vp).Current.Value ?? "";
            if (string.IsNullOrEmpty(text)) text = info.Name ?? "";
            if (string.IsNullOrEmpty(text)) text = info.HelpText ?? "";

            string id = info.AutomationId ?? "";
            bool clickable = el.TryGetCurrentPattern(InvokePattern.Pattern, out _)
                          || el.TryGetCurrentPattern(TogglePattern.Pattern, out _)
                          || el.TryGetCurrentPattern(SelectionItemPattern.Pattern, out _);

            var r = info.BoundingRectangle;
            bool hasFrame = !r.IsEmpty && r.Width > 0 && r.Height > 0;
            int[] bounds = hasFrame
                ? new[] { (int)r.Left, (int)r.Top, (int)r.Right, (int)r.Bottom }
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

    // "ControlType.Button" → "Button"; the agent reasons over short role names.
    static string TrimControlType(string? programmaticName)
    {
        if (string.IsNullOrEmpty(programmaticName)) return "";
        int dot = programmaticName.LastIndexOf('.');
        return dot >= 0 ? programmaticName[(dot + 1)..] : programmaticName;
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
