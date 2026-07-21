using System.Runtime.InteropServices;

namespace Abacad;

// Synthesizes mouse and keyboard input via SendInput. Coordinates are physical
// screen pixels — the same space UIA BoundingRectangle reports and the screen
// capture uses (the process is PerMonitorV2 DPI-aware), so a node's bounds map
// straight to a click point with no conversion. Mirrors macos/InputInjection.swift.
static class InputInjection
{
    // MARK: Mouse

    public static void Click(int x, int y, string button = "left", int count = 1,
                             IReadOnlyList<string>? modifiers = null)
    {
        var mods = ModifierVks(modifiers);
        foreach (var m in mods) Key(m, down: true);
        MoveTo(x, y);
        bool right = button.Equals("right", StringComparison.OrdinalIgnoreCase);
        uint down = right ? MOUSEEVENTF_RIGHTDOWN : MOUSEEVENTF_LEFTDOWN;
        uint up = right ? MOUSEEVENTF_RIGHTUP : MOUSEEVENTF_LEFTUP;
        for (int i = 0; i < Math.Max(1, count); i++) { Mouse(x, y, down); Mouse(x, y, up); }
        for (int i = mods.Count - 1; i >= 0; i--) Key(mods[i], down: false);
    }

    public static void RightClick(int x, int y) => Click(x, y, "right");

    /// Press at start, hold for `holdMs`, release — a press-and-hold at one point.
    public static void LongPress(int x, int y, int holdMs)
    {
        MoveTo(x, y);
        Mouse(x, y, MOUSEEVENTF_LEFTDOWN);
        Thread.Sleep(Math.Max(0, holdMs));
        Mouse(x, y, MOUSEEVENTF_LEFTUP);
    }

    /// Press at (x1,y1), interpolate to (x2,y2) over durationMs, release.
    public static void Drag(int x1, int y1, int x2, int y2, int durationMs,
                            IReadOnlyList<string>? modifiers = null)
    {
        var mods = ModifierVks(modifiers);
        foreach (var m in mods) Key(m, down: true);
        MoveTo(x1, y1);
        Mouse(x1, y1, MOUSEEVENTF_LEFTDOWN);
        int steps = Math.Max(1, Math.Min(60, durationMs / 8));
        int perStep = Math.Max(0, durationMs) / steps;
        for (int i = 1; i <= steps; i++)
        {
            double t = (double)i / steps;
            MoveTo(x1 + (int)((x2 - x1) * t), y1 + (int)((y2 - y1) * t));
            if (perStep > 0) Thread.Sleep(perStep);
        }
        Mouse(x2, y2, MOUSEEVENTF_LEFTUP);
        for (int i = mods.Count - 1; i >= 0; i--) Key(mods[i], down: false);
    }

    /// Scroll by a wheel delta at a point. Positive dy scrolls content up. Units are
    /// wheel notches (WHEEL_DELTA each); scroll lands at the current pointer, so we
    /// move there first.
    public static void Scroll(int x, int y, int dx, int dy)
    {
        MoveTo(x, y);
        if (dy != 0) Mouse(x, y, MOUSEEVENTF_WHEEL, (uint)(dy * WHEEL_DELTA));
        if (dx != 0) Mouse(x, y, MOUSEEVENTF_HWHEEL, (uint)(dx * WHEEL_DELTA));
    }

    // Composite primitives (single pointer).
    public static void PointerDown(int x, int y, string button = "left")
    {
        MoveTo(x, y);
        Mouse(x, y, button.Equals("right", StringComparison.OrdinalIgnoreCase)
            ? MOUSEEVENTF_RIGHTDOWN : MOUSEEVENTF_LEFTDOWN);
    }

    // A move with a button held auto-drags on Windows, so this covers move & drag.
    public static void PointerMove(int x, int y) => MoveTo(x, y);

    public static void PointerUp(int x, int y, string button = "left")
        => Mouse(x, y, button.Equals("right", StringComparison.OrdinalIgnoreCase)
            ? MOUSEEVENTF_RIGHTUP : MOUSEEVENTF_LEFTUP);

    // MARK: Keyboard

    /// Press a chord: modifiers held while the main key(s) go down then up. Returns
    /// false if no main (non-modifier) key was recognized.
    public static bool PressChord(IReadOnlyList<string> keys)
    {
        var mods = new List<ushort>();
        var mains = new List<ushort>();
        foreach (var k in keys)
        {
            if (KeyMap.Modifier(k) is { } m) mods.Add(m);
            else if (KeyMap.KeyCode(k) is { } kc) mains.Add(kc);
        }
        if (mains.Count == 0) return false;
        foreach (var m in mods) Key(m, down: true);
        foreach (var kc in mains) Key(kc, down: true, KeyMap.IsExtended(kc));
        for (int i = mains.Count - 1; i >= 0; i--) Key(mains[i], down: false, KeyMap.IsExtended(mains[i]));
        for (int i = mods.Count - 1; i >= 0; i--) Key(mods[i], down: false);
        return true;
    }

    /// Type a Unicode string as keystrokes (input_text / composite `type`).
    public static void TypeText(string text)
    {
        foreach (var ch in text) { UniChar(ch, down: true); UniChar(ch, down: false); }
    }

    // Named key down/up for composite key_down / key_up (accepts modifiers too).
    public static void KeyByName(string name, bool down)
    {
        if (KeyMap.KeyCode(name) is { } kc) Key(kc, down, KeyMap.IsExtended(kc));
        else if (KeyMap.Modifier(name) is { } m) Key(m, down);
    }

    // MARK: SendInput plumbing

    static List<ushort> ModifierVks(IEnumerable<string>? names)
    {
        var list = new List<ushort>();
        if (names is null) return list;
        foreach (var n in names) if (KeyMap.Modifier(n) is { } m) list.Add(m);
        return list;
    }

    static void MoveTo(int x, int y) => Mouse(x, y, MOUSEEVENTF_MOVE);

    static void Mouse(int x, int y, uint flags, uint data = 0)
    {
        var (nx, ny) = Normalize(x, y);
        var input = new INPUT
        {
            type = INPUT_MOUSE,
            u = new InputUnion
            {
                mi = new MOUSEINPUT
                {
                    dx = nx,
                    dy = ny,
                    mouseData = data,
                    dwFlags = flags | MOUSEEVENTF_ABSOLUTE | MOUSEEVENTF_VIRTUALDESK,
                    time = 0,
                    dwExtraInfo = IntPtr.Zero,
                }
            }
        };
        SendInput(1, new[] { input }, Marshal.SizeOf<INPUT>());
    }

    static void Key(ushort vk, bool down, bool extended = false)
    {
        uint flags = down ? 0u : KEYEVENTF_KEYUP;
        if (extended) flags |= KEYEVENTF_EXTENDEDKEY;
        SendKeyboard(new KEYBDINPUT { wVk = vk, wScan = 0, dwFlags = flags, time = 0, dwExtraInfo = IntPtr.Zero });
    }

    static void UniChar(char c, bool down)
    {
        uint flags = KEYEVENTF_UNICODE | (down ? 0u : KEYEVENTF_KEYUP);
        SendKeyboard(new KEYBDINPUT { wVk = 0, wScan = c, dwFlags = flags, time = 0, dwExtraInfo = IntPtr.Zero });
    }

    static void SendKeyboard(KEYBDINPUT ki)
    {
        var input = new INPUT { type = INPUT_KEYBOARD, u = new InputUnion { ki = ki } };
        SendInput(1, new[] { input }, Marshal.SizeOf<INPUT>());
    }

    // Map physical-pixel screen coords to SendInput's 0..65535 absolute space over
    // the whole virtual desktop (physical, since the process is DPI-aware).
    static (int, int) Normalize(int x, int y)
    {
        int vx = GetSystemMetrics(SM_XVIRTUALSCREEN);
        int vy = GetSystemMetrics(SM_YVIRTUALSCREEN);
        int vw = GetSystemMetrics(SM_CXVIRTUALSCREEN);
        int vh = GetSystemMetrics(SM_CYVIRTUALSCREEN);
        int nx = (int)Math.Round((x - vx) * 65535.0 / Math.Max(1, vw - 1));
        int ny = (int)Math.Round((y - vy) * 65535.0 / Math.Max(1, vh - 1));
        return (nx, ny);
    }

    // --- Win32 constants ---
    const uint INPUT_MOUSE = 0, INPUT_KEYBOARD = 1;
    const uint MOUSEEVENTF_MOVE = 0x0001, MOUSEEVENTF_LEFTDOWN = 0x0002, MOUSEEVENTF_LEFTUP = 0x0004;
    const uint MOUSEEVENTF_RIGHTDOWN = 0x0008, MOUSEEVENTF_RIGHTUP = 0x0010;
    const uint MOUSEEVENTF_WHEEL = 0x0800, MOUSEEVENTF_HWHEEL = 0x1000;
    const uint MOUSEEVENTF_ABSOLUTE = 0x8000, MOUSEEVENTF_VIRTUALDESK = 0x4000;
    const uint KEYEVENTF_EXTENDEDKEY = 0x0001, KEYEVENTF_KEYUP = 0x0002, KEYEVENTF_UNICODE = 0x0004;
    const int WHEEL_DELTA = 120;
    const int SM_XVIRTUALSCREEN = 76, SM_YVIRTUALSCREEN = 77, SM_CXVIRTUALSCREEN = 78, SM_CYVIRTUALSCREEN = 79;

    [StructLayout(LayoutKind.Sequential)]
    struct INPUT { public uint type; public InputUnion u; }

    [StructLayout(LayoutKind.Explicit)]
    struct InputUnion
    {
        [FieldOffset(0)] public MOUSEINPUT mi;
        [FieldOffset(0)] public KEYBDINPUT ki;
    }

    [StructLayout(LayoutKind.Sequential)]
    struct MOUSEINPUT
    {
        public int dx; public int dy; public uint mouseData;
        public uint dwFlags; public uint time; public IntPtr dwExtraInfo;
    }

    [StructLayout(LayoutKind.Sequential)]
    struct KEYBDINPUT
    {
        public ushort wVk; public ushort wScan; public uint dwFlags;
        public uint time; public IntPtr dwExtraInfo;
    }

    [DllImport("user32.dll", SetLastError = true)]
    static extern uint SendInput(uint nInputs, INPUT[] pInputs, int cbSize);

    [DllImport("user32.dll")]
    static extern int GetSystemMetrics(int nIndex);
}
