using System;
using System.Collections.Generic;
using System.Linq;

namespace Abacad;

/// <summary>
/// The device-side capability ceiling: which interfaces this PC is willing to
/// expose, decided here rather than by the relay.
///
/// The server keeps its own per-device set and the effective surface is the
/// intersection. The difference is who you have to trust: the server's set
/// protects you from a misbehaving agent, this one protects you from a
/// misbehaving <em>server</em> — a relay that is compromised, out of date, or
/// simply run by somebody else cannot switch a capability back on here, because
/// <see cref="Agent"/> refuses the command regardless of what it is told.
///
/// That is why the check sits in front of the dispatcher instead of being
/// trusted to the wire. In normal operation it never fires, because the relay
/// already declined to send — and that redundancy is the point, not waste.
///
/// Honest limit: it cannot constrain a capability that already grants code
/// execution as this user. Turning file transfer off is meaningful; leaving it
/// on and expecting the rest to hold is not.
/// </summary>
public static class Capabilities
{
    /// The vocabulary, mirroring the server's protocol.Capabilities.
    public static readonly string[] All =
    {
        "screenshot", "tap", "long_press", "swipe", "input_text",
        "back", "home", "recents",
        "click", "right_click", "drag", "scroll", "press_keys", "composite",
        "execute", "push_file", "pull_file", "screen_recording",
        "tunnel", "ssh", "vnc",
    };

    /// "Everything, including capabilities added in later versions." An
    /// unconfigured PC reports this rather than enumerating <see cref="All"/>,
    /// so it does not pin itself to the verb list of the version it shipped with.
    public const string Wildcard = "*";

    /// Stored sentinel for "expose nothing" — Prefs cannot distinguish an empty
    /// string from a missing value, and those mean opposite things here (no
    /// config is the wildcard; an empty set refuses everything). Without an
    /// explicit marker the most restrictive setting would read back as the most
    /// permissive one.
    const string EmptyMarker = "none";

    static readonly object Gate = new();
    static bool _wildcard = true; // no config yet => expose everything, as before
    static HashSet<string> _enabled = new();
    static readonly List<Action> Listeners = new();

    /// Reads the persisted set. Call before connecting: the ceiling must be in
    /// force before the first command arrives, and it is reported on connect.
    public static void Load()
    {
        var raw = Prefs.Capabilities;
        lock (Gate)
        {
            if (string.IsNullOrEmpty(raw)) return; // never configured => wildcard
            ApplyLocked(raw.Split(',').Select(s => s.Trim()).Where(s => s.Length > 0).ToList());
        }
    }

    /// Whether this PC exposes <paramref name="name"/>.
    public static bool Allows(string name)
    {
        lock (Gate) return _wildcard || _enabled.Contains(name);
    }

    /// What this PC advertises: the full set, always, so the latest frame is the
    /// whole truth and no delta can drift. The wildcard stays a wildcard rather
    /// than being expanded, so a newer server keeps granting capabilities this
    /// build has never heard of.
    public static List<string> Report()
    {
        lock (Gate)
        {
            return _wildcard ? new List<string> { Wildcard } : All.Where(_enabled.Contains).ToList();
        }
    }

    /// The concrete set, expanding the wildcard, for rendering checkboxes.
    public static List<string> EnabledList()
    {
        lock (Gate) return All.Where(n => _wildcard || _enabled.Contains(n)).ToList();
    }

    /// Turns one capability on or off, persists, and notifies listeners.
    /// Switching one off while in wildcard mode first materializes the wildcard
    /// into the concrete set, so "everything except X" is expressible.
    public static void Toggle(string name, bool on)
    {
        List<Action> subs;
        lock (Gate)
        {
            if (_wildcard)
            {
                _wildcard = false;
                _enabled = new HashSet<string>(All);
            }
            if (on) _enabled.Add(name); else _enabled.Remove(name);
            SaveLocked();
            subs = new List<Action>(Listeners);
        }
        foreach (var fn in subs) fn();
    }

    /// Replaces the whole set. The tray menu only ever toggles one at a time, but
    /// the CLI's --all / --none / --only set the ceiling in one move, and doing
    /// that as a sequence of toggles would persist and broadcast a half-applied
    /// set on the way through.
    public static void Set(List<string> names)
    {
        List<Action> subs;
        lock (Gate)
        {
            ApplyLocked(names);
            SaveLocked();
            subs = new List<Action>(Listeners);
        }
        foreach (var fn in subs) fn();
    }

    /// Registers a change callback (the agent re-reports; the UI repaints).
    public static void OnChange(Action fn)
    {
        lock (Gate) Listeners.Add(fn);
    }

    /// Composite step op -> the capabilities it needs.
    ///
    /// composite is the one verb that performs other verbs, so authorizing only
    /// the outer method would let a permitted composite do everything the
    /// ceiling forbids. Unknown ops are refused, not waved through, so a newer
    /// server cannot slip an unmapped action past a ceiling this build predates.
    static readonly Dictionary<string, string[]> CompositeOps = new()
    {
        ["pointer_down"] = new[] { "click", "drag" },
        ["pointer_move"] = new[] { "click", "drag" },
        ["pointer_up"] = new[] { "click", "drag" },
        ["click"] = new[] { "click" },
        ["tap"] = new[] { "tap" },
        ["long_press"] = new[] { "long_press" },
        ["swipe"] = new[] { "swipe" },
        ["drag"] = new[] { "drag" },
        ["scroll"] = new[] { "scroll" },
        ["key_down"] = new[] { "press_keys" },
        ["key_up"] = new[] { "press_keys" },
        ["type"] = new[] { "input_text" },
        ["screenshot"] = new[] { "screenshot" },
        ["wait"] = Array.Empty<string>(),
    };

    /// Authorizes one inbound command. Returns null when allowed, else the
    /// reason to refuse with.
    public static string? Refusal(string method, Dictionary<string, object?> p)
    {
        // Stopping is never blocked. vnc and screen_recording multiplex
        // start/stop through one method, so refusing the stop would strand the
        // very session the operator just revoked. A ceiling prevents things
        // starting, never ceasing.
        if ((method == "vnc" || method == "screen_recording")
            && p.TryGetValue("action", out var a) && (a as string) == "stop")
        {
            return null;
        }
        if (!Allows(method))
        {
            return $"{method} is turned off on this device — re-enable it in the abacad window";
        }
        if (method != "composite") return null;
        if (!p.TryGetValue("steps", out var sv) || sv is not List<object?> steps) return null;
        for (var i = 0; i < steps.Count; i++)
        {
            if (steps[i] is not Dictionary<string, object?> step) return $"composite: step {i} is not an object";
            if (step.TryGetValue("op", out var ov) is false || ov is not string op) return $"composite: step {i} has no op";
            if (!CompositeOps.TryGetValue(op, out var needed)) return $"composite: step {i} has unknown op \"{op}\"";
            foreach (var need in needed)
            {
                if (!Allows(need)) return $"composite: step {i} ({op}): {need} is turned off on this device";
            }
        }
        return null;
    }

    /// Authorizes a tunnel dial.
    ///
    /// The device sees only a host:port, not which server-side consumer asked,
    /// so it cannot distinguish the SSH jump from a plain /connect the way the
    /// relay can. It infers: the jump always targets this machine's own sshd, so
    /// a loopback :22 dial is allowed by EITHER ssh or tunnel, and anything else
    /// needs tunnel. That mirrors the real relationship — a tunnel can reach
    /// port 22 on its own, so treating them as independent would be a fiction.
    public static string? TunnelRefusal(string host, int port)
    {
        var loopback = host is "localhost" or "127.0.0.1" or "::1";
        if (port == 22 && loopback)
        {
            return Allows("ssh") || Allows("tunnel") ? null : "ssh is turned off on this device";
        }
        return Allows("tunnel") ? null : "tunnel is turned off on this device";
    }

    static void ApplyLocked(List<string> names)
    {
        if (names.Contains(Wildcard))
        {
            _wildcard = true;
            _enabled = new HashSet<string>();
            return;
        }
        _wildcard = false;
        _enabled = new HashSet<string>(names.Where(n => n != EmptyMarker));
    }

    static void SaveLocked()
    {
        if (_wildcard)
        {
            Prefs.Capabilities = Wildcard;
            return;
        }
        var on = All.Where(_enabled.Contains).ToList();
        // See EmptyMarker: an empty string would read back as "never
        // configured", turning "expose nothing" into "expose everything".
        Prefs.Capabilities = on.Count == 0 ? EmptyMarker : string.Join(",", on);
    }
}
