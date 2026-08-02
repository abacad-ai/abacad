namespace Abacad;

// `abacad capabilities` — the device-side control over what this PC exposes.
//
//   abacad capabilities                       # show what is on and off
//   abacad capabilities --disable files,ssh   # turn groups or names off
//   abacad capabilities --enable screenshot   # turn them back on
//   abacad capabilities --only screenshot     # exactly this, nothing else
//   abacad capabilities --none                # expose nothing
//   abacad capabilities --all                 # expose everything (the default)
//
// The tray menu has had these switches all along; a PC with no interactive
// session had no way to set them. Flags and group names are identical to the
// Linux and macOS clients, so the same command means the same thing everywhere.
//
// Changes are written to the DPAPI-encrypted per-user store and take effect at
// the next start of whichever client is running.
static class CapabilitiesCommand
{
    // The same groupings the dashboard shows, so someone moving between the two
    // is choosing the same things by the same names.
    static readonly Dictionary<string, string[]> Groups = new()
    {
        ["observe"] = ["screenshot", "screen_recording"],
        ["control"] =
        [
            "tap", "long_press", "swipe", "input_text",
            "back", "home", "recents",
            "click", "right_click", "drag", "scroll", "press_keys", "composite",
        ],
        ["files"] = ["push_file", "pull_file"],
        ["execute"] = ["execute"],
        ["live"] = ["vnc"],
        ["tunnel"] = ["tunnel"],
        ["ssh"] = ["ssh"],
    };

    public static int Run(string[] args)
    {
        string enable = "", disable = "", only = "";
        bool all = false, none = false;

        for (var i = 0; i < args.Length; i++)
        {
            var a = args[i];
            string? inline = null;
            var eq = a.IndexOf('=');
            if (a.StartsWith("--") && eq > 0)
            {
                inline = a[(eq + 1)..];
                a = a[..eq];
            }

            switch (a)
            {
                case "--all": all = true; break;
                case "--none": none = true; break;
                case "--enable":
                case "--disable":
                case "--only":
                    var v = inline;
                    if (v is null)
                    {
                        if (i + 1 >= args.Length) return Fail($"{a} needs a value");
                        v = args[++i];
                    }
                    if (a == "--enable") enable = v;
                    else if (a == "--disable") disable = v;
                    else only = v;
                    break;
                case "--help":
                case "-h":
                    Usage();
                    return 0;
                default:
                    return Fail($"unexpected argument \"{args[i]}\"");
            }
        }
        if (all && none) return Fail("--all and --none contradict each other");

        Capabilities.Load();

        var changed = false;
        if (all)
        {
            Capabilities.Set([Capabilities.Wildcard]);
            changed = true;
        }
        else if (none)
        {
            Capabilities.Set([]);
            changed = true;
        }
        else if (only.Length > 0)
        {
            if (!Expand(only, out var names, out var err)) return Fail(err);
            Capabilities.Set(names);
            changed = true;
        }

        foreach (var (list, on) in new[] { (enable, true), (disable, false) })
        {
            if (list.Length == 0) continue;
            if (!Expand(list, out var names, out var err)) return Fail(err);
            foreach (var n in names) Capabilities.Toggle(n, on);
            changed = true;
        }

        PrintStatus(changed);
        return 0;
    }

    /// Resolves a comma-separated list of group and capability names, failing on
    /// anything unrecognized rather than silently ignoring it — a typo that
    /// quietly did nothing would leave someone believing they had turned
    /// something off when they had not.
    static bool Expand(string list, out List<string> names, out string error)
    {
        names = [];
        error = "";
        foreach (var raw in list.Split(','))
        {
            var name = raw.Trim();
            if (name.Length == 0) continue;
            if (Groups.TryGetValue(name, out var members)) names.AddRange(members);
            else if (Capabilities.All.Contains(name)) names.Add(name);
            else
            {
                error = $"unknown capability or group \"{name}\" (groups: {string.Join(", ", GroupNames())})";
                return false;
            }
        }
        return true;
    }

    static IEnumerable<string> GroupNames() => Groups.Keys.OrderBy(k => k);

    static void PrintStatus(bool changed)
    {
        var on = Capabilities.EnabledList().ToHashSet();
        Console.WriteLine("This PC exposes:");
        foreach (var c in Capabilities.All)
            Console.WriteLine($"  {(on.Contains(c) ? " on" : "off")}  {c}");

        if (changed)
        {
            Console.WriteLine();
            Console.WriteLine("Saved. Restart the abacad client for this to take effect,");
            Console.WriteLine("and it will be reported to the relay on reconnect.");
        }
        // Say the sharp part out loud, in the place someone is actually making the
        // decision. A tunnel can dial this machine's own sshd, so "ssh off, tunnel
        // on" is not the closed door it looks like.
        if (on.Contains("tunnel") && !on.Contains("ssh"))
        {
            Console.WriteLine();
            Console.WriteLine("Note: ssh is off but tunnel is on — a tunnel can dial this machine's");
            Console.WriteLine("own port 22 directly, so ssh is still reachable. Disable tunnel too.");
        }
    }

    static void Usage()
    {
        Console.WriteLine($"""
            usage: abacad capabilities [--enable X] [--disable X] [--only X] [--all] [--none]

            Groups: {string.Join(", ", GroupNames())}
            Names:  {string.Join(", ", Capabilities.All)}
            """);
    }

    static int Fail(string message)
    {
        Console.Error.WriteLine($"abacad: {message}");
        return 1;
    }
}
