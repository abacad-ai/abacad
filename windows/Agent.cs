namespace Abacad;

// One activity-trail entry (a command outcome, a state change, a diagnostic).
sealed record ActivityLine(DateTime Ts, string Text);

// Coordinator: owns the socket, the command dispatcher, and the tunnel, and
// bridges them to the tray UI. The command path runs off the UI thread (SendInput/
// UIA/GDI are all callable from a background thread); the tray marshals the
// connection-state event back to the UI thread itself. Mirrors the Agent in
// macos/AbacadApp.swift.
sealed class Agent
{
    public event Action<bool>? ConnectedChanged;
    // Fired on any awareness change (activity, pause, watched, recording). The UI
    // subscribes and marshals a repaint onto the UI thread.
    public event Action? StatusChanged;

    public bool Connected { get; private set; }
    public string ServerUrl { get; private set; } = Prefs.ServerUrl;

    // Awareness state — the consent surface for the person at this PC.
    public bool Paused { get; private set; }
    public bool Watched { get; private set; }
    public bool Recording { get; private set; }
    public string? LastMethod { get; private set; }
    DateTime _lastCommandAt = DateTime.MinValue;

    // True when an agent ran a command in the last few seconds: a distinct
    // "Controlling now" state vs. an idle "Connected". Computed, so the UI polls
    // it on a 1s timer to let it decay.
    public bool Controlling => Connected && !Paused && (DateTime.UtcNow - _lastCommandAt).TotalSeconds < 6;

    readonly List<ActivityLine> _lines = new();
    readonly object _linesLock = new();
    public IReadOnlyList<ActivityLine> Lines
    {
        get { lock (_linesLock) return _lines.ToArray(); }
    }

    readonly WebSocketClient _ws = new();
    readonly CommandDispatcher _dispatcher = new();
    readonly Tunnel _tunnel = new();

    public Agent()
    {
        _dispatcher.Blobs = BlobClient.FromServerUrl(ServerUrl);
        // Load the ceiling before the socket exists: it must be in force before
        // the first command can arrive, and it is reported on connect.
        Abacad.Capabilities.Load();
        // Re-report when it changes locally, so the dashboard and the relay's
        // gate converge without waiting for a reconnect.
        Abacad.Capabilities.OnChange(SendCapabilities);
        _tunnel.SendFrame = data => _ws.Send(data);
        _ws.OnStateChange = up =>
        {
            Connected = up;
            Event(up ? "• connected" : "• disconnected");
            // Declare this PC's ceiling immediately. Until it lands the relay
            // has only the value from a previous session, so reporting first
            // thing keeps that window as short as the socket allows. Advisory
            // only — HandleText enforces it either way.
            if (up) SendCapabilities();
            ConnectedChanged?.Invoke(up);
        };
        _ws.OnText = HandleText;
        _ws.OnBinary = data => _tunnel.Handle(data);
    }

    /// Declares what this PC exposes. Always the FULL set — a delta needs both
    /// ends to agree on a starting point and they cannot after a reconnect or a
    /// dropped frame, so sending everything makes the latest frame the whole
    /// truth. Tagged by type, so relays predating it ignore it harmlessly.
    ///
    /// Fire-and-forget, and safe: this is an advisory mirror for the dashboard
    /// and the relay's gate, not the enforcement. If it never arrives this PC
    /// still refuses the same commands; the relay just does not know why yet.
    void SendCapabilities()
    {
        _ws.Send(Json.String(new Dictionary<string, object?>
        {
            ["type"] = "capabilities",
            ["capabilities"] = Abacad.Capabilities.Report().Cast<object?>().ToList(),
        }));
    }

    // Toggle the soft-kill pause (from the settings window). While paused every
    // incoming command is rejected locally; only the window can clear it.
    public void SetPaused(bool p)
    {
        if (Paused == p) return;
        Paused = p;
        Event(p ? "⏸ control paused by device operator" : "▶ control resumed");
    }

    void Event(string text)
    {
        lock (_linesLock)
        {
            _lines.Add(new ActivityLine(DateTime.Now, text));
            if (_lines.Count > 40) _lines.RemoveRange(0, _lines.Count - 40);
        }
        StatusChanged?.Invoke();
    }

    void NoteCommand(string method)
    {
        LastMethod = method;
        _lastCommandAt = DateTime.UtcNow;
        StatusChanged?.Invoke();
    }

    void SetWatched(bool w) { if (Watched != w) { Watched = w; Event(w ? "👁 live view started — screen being watched" : "live view ended"); } }
    void SetRecording(bool r) { if (Recording != r) { Recording = r; Event(r ? "● screen recording started" : "screen recording stopped"); } }

    // Reflect live-view / recording sessions, inferred from the command verbs.
    void UpdateAwareness(string method, Dictionary<string, object?> p)
    {
        var action = p.TryGetValue("action", out var av) ? av as string : null;
        if (method == "vnc") { if (action == "start") SetWatched(true); else if (action == "stop") SetWatched(false); }
        else if (method == "screen_recording") { if (action == "start") SetRecording(true); else if (action == "stop") SetRecording(false); }
    }

    // ---- self-enrollment ----

    /// Fired whenever the enrollment display changes (device id, claim code,
    /// claiming account, or an error). The window subscribes and repaints.
    public event Action? EnrollmentChanged;

    public string DeviceId { get; private set; } = Prefs.DeviceId;
    public string ClaimCode { get; private set; } = "";
    public string ClaimedBy { get; private set; } = "";
    public string RelayUrl { get; private set; } =
        Prefs.RelayUrl.Length == 0 ? Enrollment.DefaultRelay : Prefs.RelayUrl;
    public bool Enrolling { get; private set; }
    public string? EnrollError { get; private set; }

    /// True until this PC has a device token — i.e. nobody has ever set it up.
    /// The token doubles as the "has been set up" flag, so there is no separate
    /// persisted bit to keep in sync (and no sentinel needed for Prefs' ""-is-
    /// missing behaviour). Registering mints the token, so this is also exactly
    /// the state in which we must not have contacted a relay.
    public bool NeedsSetup { get; private set; } =
        Prefs.DeviceToken.Length == 0 && Prefs.ServerUrl.Length == 0;

    readonly HttpClient _enrollHttp = new() { Timeout = TimeSpan.FromSeconds(30) };
    CancellationTokenSource? _enrollCts;

    /// Dial the stored server URL on launch. With a stored token but no URL,
    /// resume enrollment: heartbeat, confirm the claim, dial. With neither, do
    /// nothing at all — allowRegister: false keeps this off the network until a
    /// human presses "Set up this PC". This matters more here than on the other
    /// clients because the installer ticks autostart by default, so Start() runs
    /// at every sign-in with nobody necessarily present.
    public void Start()
    {
        if (!string.IsNullOrEmpty(ServerUrl)) { _ws.Connect(ServerUrl); return; }
        StartEnrollment(allowRegister: false);
    }

    public void StartEnrollment(bool allowRegister)
    {
        _enrollCts?.Cancel();
        _enrollCts = new CancellationTokenSource();
        _ = EnrollAsync(_enrollCts.Token, allowRegister);
    }

    /// Register -> wait to be claimed -> connect. Bounded to two passes: the
    /// second exists only for a stored credential that turned out to be dead, so
    /// we discard it and enroll afresh in-process rather than making someone
    /// reinstall.
    ///
    /// <paramref name="allowRegister"/> gates the one call that contacts a relay
    /// this PC has never spoken to. False on the launch path, true only where a
    /// human asked: the setup button, ForgetEnrollment, ChangeRelay. The gate is
    /// inside the loop because the second pass re-registers on its own and must
    /// inherit the same permission as the first.
    async Task EnrollAsync(CancellationToken ct, bool allowRegister)
    {
        Enrolling = true;
        EnrollError = null;
        EnrollmentChanged?.Invoke();

        for (var attempt = 0; attempt < 2 && !ct.IsCancellationRequested; attempt++)
        {
            var relay = Enrollment.Normalize(RelayUrl);
            var token = Prefs.DeviceToken;

            if (token.Length == 0)
            {
                if (!allowRegister)
                {
                    Enrolling = false;
                    EnrollmentChanged?.Invoke();
                    return;
                }
                try
                {
                    var reg = await Enrollment.RegisterAsync(
                        _enrollHttp, relay, "windows", Enrollment.DefaultDeviceName(), AppVersion.Current);
                    // Enrollment's calls don't take the token, so cancellation can
                    // only be observed after the await returns. Dropping the
                    // credential here does leave an unclaimed row on the relay
                    // (reaped after an hour) — the alternative is writing it over
                    // the credential the superseding run just persisted, which
                    // orphans a device someone may already have claimed.
                    if (ct.IsCancellationRequested) return;
                    token = reg.DeviceToken;
                    Prefs.RelayUrl = relay;
                    Prefs.DeviceId = reg.DeviceId;
                    Prefs.DeviceToken = reg.DeviceToken;
                    DeviceId = reg.DeviceId;
                    ClaimCode = reg.ClaimCode;
                    NeedsSetup = false;
                    EnrollmentChanged?.Invoke();
                }
                catch (Enrollment.NotSupportedException)
                {
                    Fail(ct, "This relay doesn't support automatic setup. Run `abacad connect`, or paste a connection URL.");
                    return;
                }
                catch (Exception)
                {
                    Fail(ct, $"Couldn't reach {relay}. Check the relay address and your network.");
                    return;
                }
            }

            var transient = 0;
            var dead = false;
            while (!ct.IsCancellationRequested && !dead)
            {
                try
                {
                    var st = await Enrollment.HeartbeatAsync(_enrollHttp, relay, token);
                    if (ct.IsCancellationRequested) return;
                    transient = 0;
                    if (st.Claimed)
                    {
                        ClaimedBy = st.ClaimedBy;
                        ClaimCode = "";
                        Enrolling = false;
                        Event("• claimed by " + (st.ClaimedBy.Length == 0 ? "your account" : st.ClaimedBy));
                        EnrollmentChanged?.Invoke();
                        // Held in memory only, NOT written to Prefs.ServerUrl:
                        // leaving it unset means the next launch re-runs the
                        // heartbeat, one cheap round trip that re-confirms who
                        // owns this PC and notices a deletion.
                        var dial = Enrollment.DeviceUrl(relay) + "?token=" + token;
                        ServerUrl = dial;
                        _dispatcher.Blobs = BlobClient.FromServerUrl(dial);
                        _ws.Connect(dial);
                        return;
                    }
                    if (st.DeviceId.Length > 0) DeviceId = st.DeviceId;
                    ClaimCode = st.ClaimCode; // the relay rotates it; we display it
                    EnrollmentChanged?.Invoke();
                    await Task.Delay(Enrollment.Interval(st.HeartbeatIn), ct);
                }
                catch (Enrollment.UnknownTokenException)
                {
                    // Superseded: the run that replaced us has already rewritten
                    // Prefs, so clearing here would destroy ITS credential.
                    if (ct.IsCancellationRequested) return;
                    // Registration reaped, or the device was deleted. Start over.
                    // NeedsSetup goes back up so that on the launch path the
                    // second pass stops at the gate and the window asks, rather
                    // than silently minting a fresh identity on a relay.
                    Prefs.ClearEnrollment();
                    DeviceId = "";
                    ClaimCode = "";
                    ClaimedBy = "";  // else the window shows "not set up" and "Claimed by …" at once
                    NeedsSetup = true;
                    Event(allowRegister
                        ? "• relay no longer recognizes this device — re-registering"
                        : "• relay no longer recognizes this device — press Set up to register again");
                    EnrollmentChanged?.Invoke();
                    dead = true;
                }
                catch (OperationCanceledException) { return; }
                catch (Exception)
                {
                    // A flaky network shouldn't abandon a setup screen someone is
                    // reading a code off; retry, but don't spin forever.
                    if (++transient >= 10)
                    {
                        Fail(ct, $"Lost contact with {relay} while waiting to be claimed.");
                        return;
                    }
                    try { await Task.Delay(TimeSpan.FromSeconds(10), ct); }
                    catch (OperationCanceledException) { return; }
                }
            }
        }
        if (!ct.IsCancellationRequested)
            Fail(ct, "The relay rejected freshly issued credentials twice.");
    }

    /// Report a terminal enrollment failure — unless this run has been
    /// superseded. Enrollment's HTTP calls don't take a CancellationToken, so a
    /// cancelled run keeps going until its in-flight request returns (up to the
    /// 30s client timeout). Without this check that late failure lands on top of
    /// the run that replaced it: "Setup didn't finish" over a live claim code.
    void Fail(CancellationToken ct, string message)
    {
        if (ct.IsCancellationRequested) return;
        Enrolling = false;
        EnrollError = message;
        // The displayed code has rotated by now; leaving it up invites someone to
        // type a code the relay will reject.
        ClaimCode = "";
        EnrollmentChanged?.Invoke();
    }

    /// Disown this PC locally and enroll again so it can be claimed by someone
    /// else. The counterpart to the ClaimedBy disclosure: if that name is a
    /// surprise, this is the way out without touching the dashboard.
    public void ForgetEnrollment()
    {
        _ws.Disconnect();
        _tunnel.CloseAll();
        Prefs.ServerUrl = "";
        ServerUrl = "";
        Prefs.ClearEnrollment();
        DeviceId = "";
        ClaimCode = "";
        ClaimedBy = "";
        // Back to un-set-up until the re-register lands. If it fails, the window
        // shows the setup button again rather than stranding this PC with an
        // error and no way to retry.
        NeedsSetup = true;
        // allowRegister: pressing "that wasn't me" is a request to re-register.
        StartEnrollment(allowRegister: true);
    }

    /// Re-enroll against a different relay. Device identity is per-relay, so
    /// pointing at a new server means becoming a new device there.
    public void ChangeRelay(string relay)
    {
        _ws.Disconnect();
        Prefs.ServerUrl = "";
        ServerUrl = "";
        RelayUrl = Enrollment.Normalize(relay);
        Prefs.RelayUrl = RelayUrl;
        Prefs.ClearEnrollment();
        DeviceId = "";
        ClaimCode = "";
        ClaimedBy = "";
        NeedsSetup = true;
        // allowRegister: typing a relay and pressing Switch is a request to
        // enroll there. The one path that may register against a relay the user
        // supplied rather than the compiled-in default.
        StartEnrollment(allowRegister: true);
    }

    public void Connect(string url)
    {
        url = url.Trim();
        Prefs.ServerUrl = url;
        ServerUrl = url;
        // Pasting a connection URL is the other way to finish setup, so it
        // retires the setup prompt just as registering does.
        if (url.Length > 0) NeedsSetup = false;
        // A manual connect is a fresh intent to allow control: clear any pause.
        SetPaused(false);
        // Rebuild the blob endpoint whenever the server URL changes, so file
        // transfer follows the socket to a new host/token.
        _dispatcher.Blobs = BlobClient.FromServerUrl(url);
        _ws.Connect(url);
    }

    public void Disconnect()
    {
        _ws.Disconnect();
        _tunnel.CloseAll();
    }

    // Parse a command frame and dispatch it; reply is correlated by id.
    void HandleText(string text)
    {
        var cmd = Json.Object(text);
        if (cmd is null) return; // malformed → no reply
        string id = cmd.Str("id");
        string method = cmd.Str("method");
        var p = cmd.TryGetValue("params", out var pv) && pv is Dictionary<string, object?> d
            ? d : new Dictionary<string, object?>();

        // Soft-kill: while the operator has paused control, reject every command
        // locally without touching the PC. The agent sees an error; only the
        // settings window clears the pause.
        if (Paused)
        {
            Event($"{method} · rejected · paused");
            _ws.Send(Json.String(new Dictionary<string, object?>
            {
                ["id"] = id, ["ok"] = false, ["error"] = "paused by device operator",
            }));
            return;
        }
        // The device-side ceiling. The relay checks the same thing before
        // sending, so in normal operation this never fires — which is exactly
        // why it must be here: it is what makes the limit hold when the relay
        // does NOT check, because it is out of date, misconfigured, or simply
        // somebody else's. Enforcement that lives only at the end which might be
        // lying is not enforcement.
        var refusal = Abacad.Capabilities.Refusal(method, p);
        if (refusal is not null)
        {
            Event($"{method} · rejected · not exposed");
            _ws.Send(Json.String(new Dictionary<string, object?>
            {
                ["id"] = id, ["ok"] = false, ["error"] = refusal,
                // Distinguishes "its owner turned this off" from "this platform
                // has no such verb"; identical to the caller otherwise.
                ["code"] = "capability_disabled",
            }));
            return;
        }
        NoteCommand(method);
        UpdateAwareness(method, p);

        _ = Task.Run(async () =>
        {
            try
            {
                var result = await _dispatcher.Execute(method, p);
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = true, ["result"] = result,
                }));
                Event($"{method} · ok");
            }
            catch (Exception e)
            {
                _ws.Send(Json.String(new Dictionary<string, object?>
                {
                    ["id"] = id, ["ok"] = false, ["error"] = e.Message,
                }));
                Event($"{method} · error · {e.Message}");
            }
        });
    }
}
