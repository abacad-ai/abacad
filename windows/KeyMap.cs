namespace Abacad;

// Maps friendly key names (as an agent would send them) to Windows virtual-key
// codes and modifier keys. US layout — documented limitation, matching the macOS
// client. Cross-platform note: "delete"/"backspace" both map to VK_BACK (the mac
// "delete" key is backspace); forward-delete is "forwarddelete".
static class KeyMap
{
    // Modifier virtual-key codes.
    const ushort VK_SHIFT = 0x10, VK_CONTROL = 0x11, VK_MENU = 0x12, VK_LWIN = 0x5B;

    /// Returns the modifier's VK code if `name` is a modifier, else null. `cmd`/
    /// `meta`/`super` map to the Windows key so a mac-shaped chord still lands.
    public static ushort? Modifier(string name) => name.ToLowerInvariant() switch
    {
        "shift" => VK_SHIFT,
        "ctrl" or "control" => VK_CONTROL,
        "opt" or "option" or "alt" => VK_MENU,
        "cmd" or "command" or "meta" or "super" or "win" => VK_LWIN,
        _ => null,
    };

    /// Returns the VK code for a non-modifier key name, else null.
    public static ushort? KeyCode(string name)
    {
        var n = name.ToLowerInvariant();
        if (Named.TryGetValue(n, out var vk)) return vk;
        if (n.Length == 1 && Char(n[0]) is { } c) return c;
        return null;
    }

    /// Extended keys (nav cluster, arrows) need the KEYEVENTF_EXTENDEDKEY flag.
    public static bool IsExtended(ushort vk) => Extended.Contains(vk);

    static readonly Dictionary<string, ushort> Named = new()
    {
        ["enter"] = 0x0D, ["return"] = 0x0D, ["tab"] = 0x09, ["space"] = 0x20,
        ["delete"] = 0x08, ["backspace"] = 0x08, ["forwarddelete"] = 0x2E,
        ["esc"] = 0x1B, ["escape"] = 0x1B,
        ["left"] = 0x25, ["up"] = 0x26, ["right"] = 0x27, ["down"] = 0x28,
        ["home"] = 0x24, ["end"] = 0x23, ["pageup"] = 0x21, ["pagedown"] = 0x22,
        ["insert"] = 0x2D,
        ["f1"] = 0x70, ["f2"] = 0x71, ["f3"] = 0x72, ["f4"] = 0x73, ["f5"] = 0x74,
        ["f6"] = 0x75, ["f7"] = 0x76, ["f8"] = 0x77, ["f9"] = 0x78, ["f10"] = 0x79,
        ["f11"] = 0x7A, ["f12"] = 0x7B,
    };

    static readonly HashSet<ushort> Extended = new()
    {
        0x25, 0x26, 0x27, 0x28, // arrows
        0x24, 0x23, 0x21, 0x22, // home/end/pageup/pagedown
        0x2D, 0x2E,             // insert / forward-delete
    };

    // US-layout single character → VK code. Letters/digits map to their ASCII
    // uppercase value; punctuation uses the VK_OEM_* codes.
    static ushort? Char(char c)
    {
        if (c is >= 'a' and <= 'z') return (ushort)char.ToUpperInvariant(c);
        if (c is >= '0' and <= '9') return c;
        return c switch
        {
            ';' => (ushort)0xBA, '=' => (ushort)0xBB, ',' => (ushort)0xBC, '-' => (ushort)0xBD,
            '.' => (ushort)0xBE, '/' => (ushort)0xBF, '`' => (ushort)0xC0, '[' => (ushort)0xDB,
            '\\' => (ushort)0xDC, ']' => (ushort)0xDD, '\'' => (ushort)0xDE,
            _ => null,
        };
    }
}
