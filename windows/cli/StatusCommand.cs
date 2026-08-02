namespace Abacad;

// `abacad status` — what is configured on this PC, without connecting to
// anything. The tray app answers this by being visible; over a remote session
// there is nothing to look at, so it gets printed.
static class StatusCommand
{
    public static int Run()
    {
        Capabilities.Load();

        Console.WriteLine($"abacad {AppVersion.Current}");
        Console.WriteLine();

        var dialUrl = Prefs.ServerUrl;
        var deviceId = Prefs.DeviceId;
        var hasToken = !string.IsNullOrWhiteSpace(Prefs.DeviceToken);

        if (!string.IsNullOrWhiteSpace(dialUrl))
            Console.WriteLine($"Paired:      yes ({RedactToken(dialUrl)})");
        else if (!string.IsNullOrWhiteSpace(deviceId) && hasToken)
        {
            Console.WriteLine($"Paired:      self-enrolled as {deviceId}");
            Console.WriteLine($"Relay:       {Prefs.RelayUrl}");
        }
        else
            Console.WriteLine("Paired:      no — run `abacad connect`");

        var exposed = Capabilities.Report();
        if (exposed.Count == 1 && exposed[0] == Capabilities.Wildcard)
            Console.WriteLine("Exposes:     everything (not configured — `abacad capabilities` to narrow)");
        else if (exposed.Count == 0)
            Console.WriteLine("Exposes:     nothing");
        else
            Console.WriteLine($"Exposes:     {exposed.Count} of {Capabilities.All.Length} — {string.Join(", ", exposed)}");

        return 0;
    }

    /// The dial URL carries the device token in its query. Never print it: this
    /// output goes into terminal scrollback, tickets and screenshots.
    static string RedactToken(string url)
    {
        var q = url.IndexOf("token=", StringComparison.Ordinal);
        if (q < 0) return url;
        var end = url.IndexOf('&', q);
        return url[..(q + "token=".Length)] + "…" + (end < 0 ? "" : url[end..]);
    }
}
