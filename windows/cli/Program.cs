namespace Abacad;

// `abacad` — the Windows CLI. Same agent as the tray app, no window.
//
//   abacad                        run the agent in the foreground (Ctrl+C to stop)
//   abacad connect [--server URL] pair this PC with a workspace
//   abacad capabilities [flags]   show or change what this PC exposes
//   abacad status                 show what is configured
//
// Unlike macOS — where screen and input permissions are bound to a bundle
// identity, so only the .app can do the driving — Windows puts no such condition
// on a console process. This really is the whole client, which is what makes it
// useful on a Server box or over a remote session where a tray icon has nowhere
// to live.
static class Program
{
    [STAThread]
    static int Main(string[] args)
    {
        switch (args.Length > 0 ? args[0] : "")
        {
            case "connect":
                return ConnectFlow.Run(args);
            case "capabilities":
                return CapabilitiesCommand.Run(args.Skip(1).ToArray());
            case "status":
                return StatusCommand.Run();
            case "version":
            case "--version":
            case "-v":
                Console.WriteLine(AppVersion.Current);
                return 0;
            case "help":
            case "--help":
            case "-h":
                Usage();
                return 0;
            case "":
                return RunAgent();
            default:
                Console.Error.WriteLine($"abacad: unknown command \"{args[0]}\"");
                Console.Error.WriteLine();
                Usage();
                return 1;
        }
    }

    /// Runs the agent until Ctrl+C. The tray app keeps this alive behind an icon
    /// and a Pause button; here the console session is the whole disclosure
    /// surface, so connection state is printed rather than drawn.
    static int RunAgent()
    {
        // No Capabilities.Load() here — Agent's constructor does it, and the
        // ceiling must be in force before the first command can arrive either way.
        var agent = new Agent();
        var stopped = new ManualResetEventSlim(false);

        agent.ConnectedChanged += up =>
        {
            Console.WriteLine(up
                ? "device online — this machine can now be viewed and controlled remotely by an agent"
                : "device offline");
        };

        Console.CancelKeyPress += (_, e) =>
        {
            // Take the Ctrl+C ourselves so the agent gets a clean disconnect
            // instead of the runtime tearing the process down mid-frame.
            e.Cancel = true;
            Console.WriteLine("stopping…");
            agent.Disconnect();
            stopped.Set();
        };

        // Self-enrollment prints its claim code in the tray app's window. There is
        // no window here, and the code is live state on the agent rather than
        // anything `abacad status` could read back, so it has to be printed as it
        // arrives — otherwise a self-enrolling PC would wait to be claimed with no
        // way to learn the code to claim it with.
        var lastShown = "";
        agent.EnrollmentChanged += () =>
        {
            if (agent.ClaimCode.Length > 0 && agent.ClaimCode != lastShown)
            {
                lastShown = agent.ClaimCode;
                Console.WriteLine($"claim code: {agent.ClaimCode}   (add this device in the console to claim it)");
            }
            if (agent.ClaimedBy.Length > 0) Console.WriteLine($"claimed by {agent.ClaimedBy}");
            if (agent.EnrollError is { } err) Console.WriteLine($"enrollment: {err}");
        };

        if (string.IsNullOrWhiteSpace(Prefs.ServerUrl) && string.IsNullOrWhiteSpace(Prefs.DeviceToken))
            Console.WriteLine("Not paired yet — registering with the relay. Or pair directly with `abacad connect`.");

        agent.Start();
        stopped.Wait();
        return 0;
    }

    static void Usage()
    {
        Console.WriteLine(
            """
            usage: abacad [command] [flags]

              (no command)             run the agent in the foreground
              connect [--server URL]   pair this PC with a workspace
              capabilities [flags]     show or change what this PC exposes
              status                   show what is configured
              version                  print the version

            The tray app is the same agent with a window and a Pause button.
            """);
    }
}
