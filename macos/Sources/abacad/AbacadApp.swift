import AbacadKit
import SwiftUI
import AppKit
import Network

// Process entry point. `abacad connect` runs the device-authorization pairing
// flow as a plain CLI and exits, before any menu-bar/SwiftUI init; a bare launch
// runs the menu-bar app. Mirrors the Windows client's Program.Main branch.
@main
enum Entry {
    static func main() {
        let args = Array(CommandLine.arguments.dropFirst())
        if args.first == "connect" {
            exit(ConnectFlow.run(args))
        }
        AbacadApp.main()
    }
}

// Menu-bar app. LSUIElement (set in Info.plist) keeps it out of the Dock; the
// only UI is the menu-bar item and its panel.
struct AbacadApp: App {
    @StateObject private var agent = Agent()

    var body: some Scene {
        MenuBarExtra {
            AgentPanel(agent: agent)
        } label: {
            // Our own mono relay mark instead of an SF Symbol; the glyph is fixed,
            // connection state is exposed via the accessibility label — which now
            // also calls out an active takeover.
            Image(nsImage: RelayMark.trayImage())
                .accessibilityLabel(
                    agent.controlling ? "abacad — controlling now"
                        : agent.connected ? "abacad — connected"
                        : "abacad — disconnected")
        }
        .menuBarExtraStyle(.window)
    }
}

// One activity-trail entry (a command outcome, a state change, a diagnostic).
struct ActivityLine: Identifiable {
    let id = UUID()
    let ts: Date
    let text: String
}

// Coordinator: owns the socket, the command dispatcher, and the tunnel, and bridges
// them to the SwiftUI panel. Not @MainActor — the command path runs off the main
// thread (CGEvent/AX/capture are all thread-safe); only @Published UI state is
// republished on main.
final class Agent: ObservableObject {
    @Published var connected = false
    @Published var serverURL: String = Prefs.serverURL
    @Published var axGranted = Permissions.accessibilityGranted
    @Published var screenGranted = Permissions.screenRecordingGranted
    @Published var launchAtLoginStatus = LaunchAtLogin.status
    @Published var launchAtLoginError: String?

    // Self-enrollment state. While unclaimed the panel shows deviceID + claimCode
    // — the two things a human reads off this Mac and types at <relay>/claim.
    // Once claimed, claimedBy names the account that took it, so a claim by
    // someone who merely read the screen isn't invisible to the person here.
    @Published var relayURL: String = Prefs.relayURL.isEmpty ? Enrollment.defaultRelay : Prefs.relayURL
    @Published var deviceID: String = Prefs.deviceID
    @Published var claimCode: String = ""
    @Published var claimedBy: String = ""
    @Published var enrolling = false
    @Published var enrollError: String?

    // True until this Mac has a device token — i.e. nobody has ever set it up.
    // The token doubles as the "has been set up" flag, so there is no separate
    // persisted bit to keep in sync. Registering is what mints the token, so
    // this is also exactly the state in which we must not have talked to a relay.
    @Published var needsSetup: Bool = Prefs.deviceToken.isEmpty && Prefs.serverURL.isEmpty

    // Awareness state — the consent surface for the person at this Mac.
    @Published var paused = false            // soft-kill: reject commands locally
    @Published var watched = false           // a live-view (VNC) session is active
    @Published var recording = false         // a screen recording is in progress
    @Published var controlling = false       // an agent ran a command in the last few seconds
    @Published var lastMethod: String?
    @Published private(set) var lines: [ActivityLine] = []

    private let ws = WebSocketClient()
    private var dispatcher = CommandDispatcher()
    private let tunnel = Tunnel()
    private var controlGen = 0
    /// The in-flight enrollment, retained so a newer request can cancel it, plus
    /// a generation stamp.
    ///
    /// Both are needed. `Enrollment.heartbeat` is a plain async call with no
    /// cancellation support, so cancelling a task suspended inside it does not
    /// resume it — the old run finishes its request seconds later and would
    /// otherwise publish that relay's claim code over the new run's, and let its
    /// `defer` lower `enrolling` while the new run is still working. The
    /// generation is what makes a superseded run's writes no-ops.
    private var enrollTask: Task<Void, Never>?
    private var enrollGen = 0

    // Power/network watchers. A sleeping Mac can't answer the relay's liveness
    // pings, so the socket dies during sleep no matter what we do — what matters
    // is coming back the moment it wakes instead of waiting for URLSession to
    // notice a connection that's been dead for hours. asleep also drives the
    // presence frames below.
    private let pathMonitor = NWPathMonitor()
    private var powerObservers: [NSObjectProtocol] = []
    // Written by the power observers (main), read from the socket's queue when a
    // reconnect re-reports presence — so it needs a lock, small as it is.
    private let asleepLock = NSLock()
    private var asleepFlag = false
    private var asleep: Bool {
        get { asleepLock.lock(); defer { asleepLock.unlock() }; return asleepFlag }
        set { asleepLock.lock(); asleepFlag = newValue; asleepLock.unlock() }
    }

    init() {
        dispatcher.blobClient = BlobClient.fromServerURL(serverURL)
        // Load the ceiling before the socket exists: it must be in force before
        // the first command can arrive, and it is reported on connect.
        Capabilities.load()
        // Re-report when it changes locally, so the dashboard and the relay's
        // gate converge without waiting for a reconnect.
        Capabilities.onChange { [weak self] in self?.sendCapabilities() }
        tunnel.sendFrame = { [weak self] data in self?.ws.send(data: data) }
        ws.onStateChange = { [weak self] up in
            guard let self else { return }
            // A fresh socket starts out assumed active server-side, so re-report
            // our real power state on every (re)connect.
            if up {
                self.sendPresence(self.asleep ? "asleep" : "active")
                // Declare this Mac's ceiling immediately. Until it lands the
                // relay has only the value from a previous session, so
                // reporting first thing keeps that window as short as the
                // socket allows. Advisory only — handle(text:) enforces it
                // either way.
                self.sendCapabilities()
            }
            DispatchQueue.main.async {
                self.connected = up
                self.event(up ? "• connected" : "• disconnected")
            }
        }
        ws.onText = { [weak self] text in self?.handle(text: text) }
        ws.onBinary = { [weak self] data in self?.tunnel.handle(data) }
        observeSystemEvents()
        if !serverURL.isEmpty || !Prefs.deviceToken.isEmpty {
            // Existing configured installs and Macs paired by the CLI need the
            // same run-at-login default as setup completed in this process.
            // The helper records an explicit opt-out, so this migration runs at
            // most once and never fights a later choice.
            Task { @MainActor in self.enableLaunchAtLoginAfterSetup() }
        }
        if !serverURL.isEmpty {
            // An explicitly configured URL (from `abacad connect`, or pasted)
            // still wins, so existing installs are untouched.
            ws.connect(urlString: serverURL)
        } else {
            // Nothing configured. Resume enrollment only — allowRegister: false
            // means that with no stored token this returns without touching the
            // network, and the panel offers "Set up this Mac" instead. Launching
            // the app is not consent to be registered with a relay; pressing the
            // button is. With a token already in hand the human has pressed it
            // before, so this is the unchanged heartbeat -> claim -> dial path.
            //
            // Only when there is something to resume. Starting a run that would
            // immediately hit the allowRegister gate would raise `enrolling` for
            // one hop, flashing "Contacting …" on a Mac contacting nothing.
            //
            // init is not @MainActor, so hop rather than calling directly.
            if !Prefs.deviceToken.isEmpty {
                Task { @MainActor in self.startEnrollment(allowRegister: false) }
            }
        }
    }

    // MARK: self-enrollment

    /// Drives the register -> wait-to-be-claimed -> connect loop. Bounded to two
    /// passes: the second exists only for the case where a stored credential
    /// turned out to be dead, so we discard it and enroll afresh in-process
    /// rather than making someone quit and relaunch.
    ///
    /// `allowRegister` gates the one call that contacts a relay we have never
    /// spoken to. It is false on the launch path and true only where a human
    /// asked for setup: `beginSetup`, `forgetEnrollment`, `changeRelay`. The gate
    /// lives here rather than at the call site because the loop re-registers on
    /// its own after `.credentialDead`, and that second pass must inherit the
    /// same permission as the first.
    @MainActor
    private func runEnrollment(allowRegister: Bool, gen: Int) async {
        enrollError = nil
        // Only the current run may lower the flag: a superseded one finishing its
        // in-flight request would otherwise clear `enrolling` mid-way through the
        // run that replaced it, flashing the setup button over a live enrollment.
        defer { if gen == enrollGen { enrolling = false } }

        for _ in 0..<2 {
            let relay = Enrollment.normalize(relayURL)
            var token = Prefs.deviceToken

            if token.isEmpty {
                guard allowRegister else { return }
                do {
                    let reg = try await Enrollment.register(
                        relay: relay, platform: "macos",
                        name: Enrollment.defaultDeviceName(), version: Enrollment.appVersion())
                    // Superseded while the request was in flight: the newer run
                    // has already cleared Prefs, so writing ANY of these would
                    // resurrect a credential for a relay we are no longer
                    // enrolling with. Must precede every write — persisting the
                    // old relay/id beside the new run's token leaves a config
                    // that heartbeats the wrong pair and wipes itself on the next
                    // launch. The relay keeps an unclaimed row; reaped in an hour.
                    guard isCurrent(gen) else { return }
                    token = reg.deviceToken
                    Prefs.relayURL = relay
                    Prefs.deviceID = reg.deviceID
                    Prefs.deviceToken = reg.deviceToken
                    deviceID = reg.deviceID
                    claimCode = reg.claimCode
                    needsSetup = false
                    enableLaunchAtLoginAfterSetup()
                } catch Enrollment.EnrollError.notSupported {
                    // Errors are gated too: a superseded run reporting a failure
                    // would stamp it over the live one, and because `defer` only
                    // lets the current generation lower `enrolling`, the panel
                    // would strand on error-with-no-button. Windows guards the
                    // same three paths via Fail(ct:).
                    guard isCurrent(gen) else { return }
                    enrollError = "This relay doesn't support automatic setup. Run `abacad connect`, or paste a connection URL below."
                    return
                } catch {
                    guard isCurrent(gen) else { return }
                    enrollError = "Couldn't reach \(relay). Check the relay address and your network."
                    return
                }
            }

            switch await waitForClaim(relay: relay, token: token, gen: gen) {
            case .claimed(let status):
                claimedBy = status.claimedBy
                claimCode = ""
                needsSetup = false
                event("• claimed by \(status.claimedBy.isEmpty ? "your account" : status.claimedBy)")
                // WebSocketClient splits ?token= into an Authorization header at
                // dial time, so the combined form is what the rest of the app
                // expects — including the Advanced "Connect" button, which reads
                // serverURL. Held in memory only, NOT written to Prefs: leaving
                // it unset means the next launch re-runs the heartbeat, which is
                // one cheap round trip that re-confirms who owns this Mac and
                // notices if the device was deleted.
                let dialURL = Enrollment.deviceURL(relay: relay) + "?token=" + token
                serverURL = dialURL
                dispatcher.blobClient = BlobClient.fromServerURL(dialURL)
                ws.connect(urlString: dialURL)
                return
            case .credentialDead:
                // Registration reaped, or the device was deleted. Start over.
                // needsSetup goes back up so that if we got here on the launch
                // path (allowRegister false) the next pass stops at the guard and
                // the panel asks before re-registering, rather than silently
                // minting a fresh identity on a relay behind the user's back.
                Prefs.clearEnrollment()
                deviceID = ""
                claimCode = ""
                claimedBy = ""   // else the panel shows "not set up" and "Claimed by …" at once
                needsSetup = true
                event(allowRegister
                    ? "• relay no longer recognizes this device — re-registering"
                    : "• relay no longer recognizes this device — press Set up to register again")
                continue
            case .cancelled:
                return
            case .failed(let message):
                enrollError = message
                return
            }
        }
        if isCurrent(gen) {
            enrollError = "The relay rejected freshly issued credentials twice."
        }
    }

    private enum ClaimOutcome {
        case claimed(Enrollment.Status)
        case credentialDead
        case failed(String)
        case cancelled
    }

    @MainActor
    private func waitForClaim(relay: String, token: String, gen: Int) async -> ClaimOutcome {
        var transientFailures = 0
        // `try? await Task.sleep` swallows the cancellation error, so the loop
        // has to test for it explicitly or a cancelled run spins at full speed.
        while isCurrent(gen) {
            do {
                let st = try await Enrollment.heartbeat(relay: relay, token: token)
                // heartbeat is not cancellation-aware, so this is the first point
                // a superseded run can notice. Return before touching any state.
                guard isCurrent(gen) else { return .cancelled }
                transientFailures = 0
                if st.claimed { return .claimed(st) }
                if !st.deviceID.isEmpty { deviceID = st.deviceID }
                claimCode = st.claimCode // the relay rotates it; we just display it
                try? await Task.sleep(nanoseconds: Enrollment.interval(st.heartbeatIn) * 1_000_000_000)
            } catch Enrollment.EnrollError.unknownToken {
                // Same check: a superseded run must not report a dead credential,
                // because the caller responds by wiping Prefs — which by now
                // holds the replacing run's token.
                guard isCurrent(gen) else { return .cancelled }
                return .credentialDead
            } catch {
                // A flaky network shouldn't abandon a setup screen someone is
                // looking at; keep retrying, but give up rather than spin forever.
                transientFailures += 1
                if transientFailures >= 10 {
                    // The loop condition tests the generation, but this early
                    // return skips it — a superseded run must not report failure.
                    guard isCurrent(gen) else { return .cancelled }
                    return .failed("Lost contact with \(relay) while waiting to be claimed.")
                }
                try? await Task.sleep(nanoseconds: 10 * 1_000_000_000)
            }
        }
        return .cancelled
    }

    /// The human asked to set this Mac up. This is the only path on a fresh
    /// install that is allowed to contact a relay, and the first thing it does is
    /// register — so it must not be reachable except from a button.
    ///
    /// The `enrolling` guard is load-bearing: `waitForClaim` is an unbounded loop
    /// with no retained Task handle, so a second press would leave two of them
    /// running, both writing `claimCode` from their own heartbeats.
    @MainActor
    func beginSetup() { startEnrollment(allowRegister: true) }

    /// Single entry point for every enrollment request. Supersedes any run
    /// already in flight rather than refusing the new one.
    ///
    /// Cancellation, not a guard: `changeRelay` and `forgetEnrollment` both wipe
    /// the stored credential before re-enrolling, so a guard would drop the new
    /// intent and leave the OLD loop heartbeating a token that no longer exists —
    /// and, on claim, dialling the relay the user just switched away from. Worse,
    /// a stale pass reaching `.credentialDead` calls `Prefs.clearEnrollment()`
    /// and destroys the credential the newer pass just registered. Windows has
    /// had `_enrollCts` for this reason; macOS was relying on nothing.
    @MainActor
    private func startEnrollment(allowRegister: Bool) {
        enrollTask?.cancel()
        enrollGen &+= 1
        let gen = enrollGen
        enrolling = true
        enrollTask = Task { @MainActor in
            await self.runEnrollment(allowRegister: allowRegister, gen: gen)
        }
    }

    /// True while this run is still the current one. Every state write in the
    /// enrollment path is conditioned on it.
    @MainActor
    private func isCurrent(_ gen: Int) -> Bool { gen == enrollGen && !Task.isCancelled }

    /// Disown this Mac locally and enroll again, so it can be claimed by someone
    /// else. The counterpart to the "claimed by" disclosure: if that name is a
    /// surprise, this is the way out without touching the dashboard.
    @MainActor
    func forgetEnrollment() {
        ws.disconnect()
        tunnel.closeAll()
        Prefs.serverURL = ""
        serverURL = ""
        Prefs.clearEnrollment()
        deviceID = ""
        claimCode = ""
        claimedBy = ""
        // Back to un-set-up until the re-register lands. If it fails, the panel
        // shows the setup button again rather than stranding this Mac with an
        // error and no way to retry.
        needsSetup = true
        // allowRegister: the human pressed "that wasn't me" — re-registering is
        // the whole point of the button, so this is consent by the same standard
        // as beginSetup.
        startEnrollment(allowRegister: true)
    }

    /// Re-enroll against a different relay (from the panel). Drops the current
    /// credential: device identity is per-relay, so pointing at a new server
    /// means becoming a new device there.
    @MainActor
    func changeRelay(to newRelay: String) {
        let relay = Enrollment.normalize(newRelay)
        ws.disconnect()
        Prefs.serverURL = ""
        serverURL = ""
        Prefs.relayURL = relay
        Prefs.clearEnrollment()
        relayURL = relay
        deviceID = ""
        claimCode = ""
        claimedBy = ""
        needsSetup = true
        // allowRegister: picking a relay and pressing Switch is a request to
        // enroll there. Note this is the one path that may register against a
        // relay the user typed rather than the compiled-in default.
        startEnrollment(allowRegister: true)
    }

    deinit {
        pathMonitor.cancel()
        for o in powerObservers { NSWorkspace.shared.notificationCenter.removeObserver(o) }
    }

    // Sleep/wake and network-path changes, the desktop counterpart to the
    // Android service's SCREEN_ON / USER_PRESENT / network-available receivers
    // (AbacadAccessibilityService.systemReceiver). Wake and a returning network
    // path both force an immediate redial; sleep reports the device asleep so the
    // dashboard can tell "asleep" from "gone" for as long as the socket lasts.
    private func observeSystemEvents() {
        let nc = NSWorkspace.shared.notificationCenter
        func on(_ name: NSNotification.Name, _ body: @escaping () -> Void) {
            powerObservers.append(nc.addObserver(forName: name, object: nil, queue: nil) { _ in body() })
        }
        on(NSWorkspace.willSleepNotification) { [weak self] in self?.setAsleep(true) }
        on(NSWorkspace.screensDidSleepNotification) { [weak self] in self?.setAsleep(true) }
        on(NSWorkspace.didWakeNotification) { [weak self] in self?.setAsleep(false) }
        on(NSWorkspace.screensDidWakeNotification) { [weak self] in self?.setAsleep(false) }
        on(NSWorkspace.sessionDidBecomeActiveNotification) { [weak self] in self?.setAsleep(false) }

        pathMonitor.pathUpdateHandler = { [weak self] path in
            guard path.status == .satisfied else { return }
            self?.ws.forceReconnect()
        }
        pathMonitor.start(queue: DispatchQueue(label: "abacad.net.path"))
    }

    private func setAsleep(_ v: Bool) {
        guard asleep != v else { return }
        asleep = v
        if v {
            // Best-effort: the send may not make it out before the machine freezes.
            sendPresence("asleep")
        } else {
            ws.forceReconnect() // no-op if the socket survived the nap
            sendPresence("active")
        }
        event(v ? "• display/system asleep" : "• awake")
    }

    // Unsolicited power-state frame; the relay records it as a display signal and
    // keeps routing commands either way (relay.DeviceConn.setActivity).
    private func sendPresence(_ state: String) {
        ws.send(text: Json.string(["type": "presence", "state": state]))
    }

    /// Declares what this Mac exposes. Always the FULL set — a delta needs both
    /// ends to agree on a starting point and they cannot after a reconnect or a
    /// dropped frame, so sending everything makes the latest frame the whole
    /// truth. Tagged like presence, so relays predating it ignore it harmlessly.
    ///
    /// Fire-and-forget, and safe: this is an advisory mirror for the dashboard
    /// and the relay's gate, not the enforcement. If it never arrives the Mac
    /// still refuses the same commands; the relay just does not know why yet.
    func sendCapabilities() {
        ws.send(text: Json.string([
            "type": "capabilities", "capabilities": Capabilities.report(),
        ]))
    }

    @MainActor
    func connect() {
        let url = serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
        Prefs.serverURL = url
        serverURL = url
        // Pasting a connection URL is the other way to finish setup, so it
        // retires the setup prompt just as registering does.
        if !url.isEmpty {
            needsSetup = false
            enableLaunchAtLoginAfterSetup()
        }
        // A manual connect is a fresh intent to allow control: clear any pause.
        setPaused(false)
        // Rebuild the blob endpoint whenever the server URL changes, so file
        // transfer follows the socket to a new host/token.
        dispatcher.blobClient = BlobClient.fromServerURL(url)
        ws.connect(urlString: url)
    }

    func disconnect() {
        ws.disconnect()
        tunnel.closeAll()
    }

    func refreshPermissions() {
        axGranted = Permissions.accessibilityGranted
        screenGranted = Permissions.screenRecordingGranted
    }

    var launchAtLoginEnabled: Bool {
        LaunchAtLogin.isRequested(launchAtLoginStatus)
    }

    var launchAtLoginNeedsApproval: Bool {
        launchAtLoginStatus == .requiresApproval
    }

    var launchAtLoginUnavailable: Bool {
        launchAtLoginStatus == .notFound
    }

    @MainActor
    func refreshLaunchAtLogin() {
        launchAtLoginStatus = LaunchAtLogin.status
    }

    @MainActor
    func setLaunchAtLogin(_ enabled: Bool) {
        do {
            try LaunchAtLogin.setEnabled(enabled)
            launchAtLoginError = nil
        } catch {
            launchAtLoginError = error.localizedDescription
        }
        refreshLaunchAtLogin()
    }

    @MainActor
    private func enableLaunchAtLoginAfterSetup() {
        do {
            try LaunchAtLogin.enableAfterSetupIfUnconfigured()
            launchAtLoginError = nil
        } catch {
            launchAtLoginError = error.localizedDescription
        }
        refreshLaunchAtLogin()
    }

    // Toggle the soft-kill pause (from the panel). While paused every incoming
    // command is rejected locally; only the panel can clear it.
    func setPaused(_ p: Bool) {
        paused = p
        event(p ? "⏸ control paused by device operator" : "▶ control resumed")
    }

    // MARK: - status helpers (always hop to main for @Published)

    private func event(_ text: String) {
        DispatchQueue.main.async {
            self.lines.append(ActivityLine(ts: Date(), text: text))
            if self.lines.count > 40 { self.lines.removeFirst(self.lines.count - 40) }
        }
    }

    private func noteCommand(_ method: String) {
        DispatchQueue.main.async {
            self.lastMethod = method
            self.controlling = true
            self.controlGen += 1
            let gen = self.controlGen
            // Decay "controlling now" a few seconds after the last command.
            DispatchQueue.main.asyncAfter(deadline: .now() + 6) {
                if self.controlGen == gen { self.controlling = false }
            }
        }
    }

    private func setWatched(_ w: Bool) {
        DispatchQueue.main.async {
            guard self.watched != w else { return }
            self.watched = w
            self.event(w ? "👁 live view started — screen being watched" : "live view ended")
        }
    }

    private func setRecording(_ r: Bool) {
        DispatchQueue.main.async {
            guard self.recording != r else { return }
            self.recording = r
            self.event(r ? "● screen recording started" : "screen recording stopped")
        }
    }

    // Reflect live-view / recording sessions, inferred from the command verbs.
    private func updateAwareness(method: String, params: [String: Any]) {
        let action = params["action"] as? String
        switch method {
        case "vnc":
            if action == "start" { setWatched(true) } else if action == "stop" { setWatched(false) }
        case "screen_recording":
            if action == "start" { setRecording(true) } else if action == "stop" { setRecording(false) }
        default:
            break
        }
    }

    // Parse a command frame and dispatch it; reply is correlated by id.
    private func handle(text: String) {
        guard let cmd = Json.object(text) else { return } // malformed → no reply
        let id = cmd["id"] as? String ?? ""
        let method = cmd.string("method")
        let params = cmd["params"] as? [String: Any] ?? [:]

        // Soft-kill: while the operator has paused control, reject every command
        // locally without touching the Mac. The agent sees an error; only the panel
        // clears the pause.
        if paused {
            event("\(method) · rejected · paused")
            ws.send(text: Json.string(["id": id, "ok": false, "error": "paused by device operator"]))
            return
        }
        // The device-side ceiling. The relay checks the same thing before
        // sending, so in normal operation this never fires — which is exactly
        // why it must be here: it is what makes the limit hold when the relay
        // does NOT check, because it is out of date, misconfigured, or simply
        // somebody else's. Enforcement that lives only at the end which might be
        // lying is not enforcement.
        if let why = Capabilities.refusal(method: method, params: params) {
            event("\(method) · rejected · not exposed")
            // The code distinguishes "its owner turned this off" from "this
            // platform has no such verb"; identical to the caller otherwise.
            ws.send(text: Json.string([
                "id": id, "ok": false, "error": why, "code": "capability_disabled",
            ]))
            return
        }
        noteCommand(method)
        updateAwareness(method: method, params: params)

        Task.detached { [weak self, ws = ws, dispatcher = dispatcher] in
            do {
                let result = try await dispatcher.execute(method: method, params: params)
                ws.send(text: Json.string(["id": id, "ok": true, "result": result]))
                self?.event("\(method) · ok")
            } catch let CmdError.message(m) {
                ws.send(text: Json.string(["id": id, "ok": false, "error": m]))
                self?.event("\(method) · error · \(m)")
            } catch {
                ws.send(text: Json.string(["id": id, "ok": false, "error": "\(error)"]))
                self?.event("\(method) · error")
            }
        }
    }
}

// The menu-bar panel. Readiness-driven: not ready (disconnected, or a required
// permission missing) leads with setup; ready leads with the live awareness
// surface (state, watched/recording flags, Pause/Disconnect, recent actions),
// with setup folded into a disclosure group.
struct AgentPanel: View {
    @ObservedObject var agent: Agent

    /// Bumped on every capability change so the switches repaint. Capabilities
    /// is a plain thread-safe store rather than observable state — it has to be
    /// readable from the socket's queue, where an ObservableObject would be the
    /// wrong tool.
    @State private var capabilityRevision = 0

    private var ready: Bool { agent.connected && agent.axGranted && agent.screenGranted }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: Theme.spaceMd) {
                stateHeader

                if agent.watched || agent.recording { awarenessFlags }

                if ready {
                    Divider()
                    recentActions
                    Divider()
                    DisclosureGroup("Setup & connection") { setupBody }
                        .font(.caption)
                    Divider()
                    DisclosureGroup("What this Mac exposes") { capabilitiesBody }
                        .font(.caption)
                } else {
                    Divider()
                    connectBody
                    Divider()
                    permissionsBody
                }

                Divider()
                RestartReadinessSection(agent: agent)
                Divider()
                Button("Quit abacad") { NSApplication.shared.terminate(nil) }
            }
            .padding(Theme.spaceLg)
        }
        .frame(width: 340)
        .frame(maxHeight: panelMaxHeight)
        .onAppear {
            agent.refreshPermissions()
            agent.refreshLaunchAtLogin()
        }
    }

    private var panelMaxHeight: CGFloat {
        let available = NSScreen.main?.visibleFrame.height ?? 800
        return max(320, min(720, available - 80))
    }

    // MARK: state header

    private var stateHeader: some View {
        let (color, title, subtitle): (Color, String, String) = {
            if agent.paused { return (Theme.warning, "Paused", "commands are being rejected on this Mac") }
            if agent.controlling { return (Theme.success, "Controlling now", "agent · \(agent.lastMethod ?? "running")") }
            if agent.connected { return (Theme.success, "Connected", "idle — no agent active") }
            return (Theme.inkSubtle, "Disconnected", "not connected")
        }()
        return HStack(alignment: .top, spacing: Theme.spaceSm) {
            Circle().fill(color).frame(width: 9, height: 9).padding(.top, 5)
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.headline)
                Text(subtitle).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if ready {
                Button(agent.paused ? "Resume" : "Pause") { agent.setPaused(!agent.paused) }
                Button("Disconnect") { agent.disconnect() }
                    .foregroundStyle(Theme.danger)
            }
        }
    }

    /// What this Mac exposes, decided here rather than by the relay.
    ///
    /// Grouped, because 21 switches is not a decision anyone makes well — the
    /// same groups the dashboard shows, so moving between the two means choosing
    /// the same things by the same names. Storage stays per verb.
    ///
    /// The copy says what each group really grants: these are not peers, and a
    /// column of identical-looking switches would imply they are.
    private var capabilitiesBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceSm) {
            Text("Turn off anything this Mac should never do. Enforced here, on the device, so it holds even if the relay is compromised.")
                .font(.caption2).foregroundStyle(.secondary)
            ForEach(Self.capabilityGroups, id: \.label) { group in
                Toggle(isOn: Binding(
                    get: { group.members.allSatisfy { Capabilities.allows($0) } },
                    set: { want in
                        group.members.forEach { Capabilities.toggle($0, on: want) }
                        capabilityRevision += 1
                    }
                )) {
                    VStack(alignment: .leading, spacing: 1) {
                        Text(group.label).font(.caption)
                        Text(group.note).font(.caption2).foregroundStyle(.secondary)
                    }
                }
                .toggleStyle(.switch)
                .controlSize(.mini)
            }
            // A tunnel dials this Mac's own sshd, so "ssh off, tunnel on" is not
            // the closed door it looks like. Say so where the choice is made.
            if Capabilities.allows("tunnel") && !Capabilities.allows("ssh") {
                Text("SSH is off but the network tunnel is on — a tunnel can reach this Mac's own port 22. Turn the tunnel off too.")
                    .font(.caption2).foregroundStyle(Theme.warning)
            }
        }
        // Read the counter so SwiftUI repaints: Capabilities is a plain store,
        // not observable state.
        .id(capabilityRevision)
    }

    private struct CapabilityGroup {
        let label: String
        let members: [String]
        let note: String
    }

    private static let capabilityGroups: [CapabilityGroup] = [
        .init(label: "See the screen", members: ["screenshot", "screen_recording"],
              note: "Also returns the text of every on-screen field, not just pixels."),
        .init(label: "Control input", members: ["click", "right_click", "drag", "scroll", "press_keys", "input_text", "composite", "tap", "long_press", "swipe"],
              note: "Effectively full control: anything that can type can open a terminal."),
        .init(label: "Read and write files", members: ["push_file", "pull_file"],
              note: "Writing files is equivalent to full control."),
        .init(label: "Live view", members: ["vnc"],
              note: "Not read-only — it carries keyboard and pointer events back."),
        .init(label: "Network tunnel", members: ["tunnel"],
              note: "The broadest setting: reaches any port this Mac can, including its own."),
        .init(label: "SSH", members: ["ssh"], note: "Reach this Mac's sshd via the jump host."),
    ]

    private var awarenessFlags: some View {
        HStack(spacing: Theme.spaceSm) {
            if agent.watched {
                Label("Screen being watched", systemImage: "eye.fill")
                    .font(.caption).foregroundStyle(Theme.warning)
            }
            if agent.recording {
                Label("Recording", systemImage: "record.circle")
                    .font(.caption).foregroundStyle(Theme.danger)
            }
        }
    }

    // MARK: recent actions

    private var recentActions: some View {
        VStack(alignment: .leading, spacing: Theme.spaceXs) {
            Text("Recent actions").font(.caption).foregroundStyle(.secondary)
            if agent.lines.isEmpty {
                Text("No activity yet.").font(.caption2).foregroundStyle(.secondary)
            } else {
                ForEach(agent.lines.suffix(10).reversed()) { line in
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text(Self.clock.string(from: line.ts))
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(.tertiary)
                        Text(line.text).font(.caption2)
                    }
                }
            }
        }
    }

    // MARK: setup (demoted when ready, primary when not)

    // While unclaimed, the two lines a human reads off this Mac and types at
    // <relay>/claim. This replaces "paste a connection URL" as the primary setup
    // step: nothing secret travels toward the device any more.
    private var claimBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceXs) {
            Text("Add this Mac").font(.caption).foregroundStyle(.secondary)
            HStack {
                Text("Device ID").font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(agent.deviceID).font(.system(.body, design: .monospaced)).textSelection(.enabled)
            }
            HStack {
                Text("Claim code").font(.caption2).foregroundStyle(.secondary)
                Spacer()
                Text(agent.claimCode)
                    .font(.system(.title3, design: .monospaced)).bold()
                    .textSelection(.enabled)
            }
            Text("Open \(Enrollment.normalize(agent.relayURL))/claim and enter both.")
                .font(.caption2).foregroundStyle(.secondary)
            Text("The code changes every few minutes. Anyone who can read these two lines can add this Mac to their account.")
                .font(.caption2).foregroundStyle(.secondary)
        }
    }

    private var connectBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceXs) {
            // Nothing has ever been set up here, so nothing has been sent to a
            // relay. Say which one this will contact before contacting it — the
            // address is editable under "Relay & advanced" and a self-hoster
            // needs to change it BEFORE registering, not after.
            if agent.needsSetup && !agent.enrolling {
                Text("This Mac hasn't been set up yet.")
                    .font(.caption).foregroundStyle(.secondary)
                Button("Set up this Mac") { agent.beginSetup() }
                Text("Registers with \(Enrollment.normalize(agent.relayURL)) and shows a code to enter there. Nothing is sent until you press it.")
                    .font(.caption2).foregroundStyle(.secondary)
                Divider()
            } else if agent.enrollError != nil && !agent.enrolling {
                // A device that already holds a token isn't needsSetup, so
                // without this it would render the error and no way to act on
                // it — the claim code shown alongside has rotated and is stale.
                Button("Try again") { agent.beginSetup() }
            } else if agent.enrolling && agent.claimCode.isEmpty && agent.enrollError == nil {
                // deviceID is seeded from Prefs, so testing it here used to hide
                // the spinner on every relaunch of a registered-but-unclaimed Mac.
                // The claim code is the thing this is waiting for, so gate on that.
                HStack(spacing: Theme.spaceXs) {
                    ProgressView().controlSize(.small)
                    Text("Contacting \(Enrollment.normalize(agent.relayURL))…").font(.caption)
                }
            } else if !agent.claimCode.isEmpty {
                claimBody
                Divider()
            }

            if let err = agent.enrollError {
                Text(err).font(.caption).foregroundStyle(.red)
            }

            // Who holds this device. Shown after a claim so a claim made by
            // someone who merely read the screen is visible to the person at the
            // keyboard, with a one-click way out.
            if !agent.claimedBy.isEmpty {
                HStack {
                    Text("Claimed by").font(.caption2).foregroundStyle(.secondary)
                    Spacer()
                    Text(agent.claimedBy).font(.caption)
                }
                Button("That wasn't me — disconnect") { agent.forgetEnrollment() }
                    .font(.caption)
            }

            DisclosureGroup("Relay & advanced") {
                VStack(alignment: .leading, spacing: Theme.spaceXs) {
                    Text("Relay").font(.caption2).foregroundStyle(.secondary)
                    HStack {
                        TextField(Enrollment.defaultRelay, text: $agent.relayURL)
                            .textFieldStyle(.roundedBorder)
                        Button("Switch") { agent.changeRelay(to: agent.relayURL) }
                    }
                    if Enrollment.normalize(agent.relayURL) == Enrollment.defaultRelay {
                        Text("On the same network as your agent? Your own relay is lower latency — abacad.ai/docs/guides/self-hosting/")
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                    Divider()
                    Text("Connection URL").font(.caption2).foregroundStyle(.secondary)
                    TextField("wss://host/device?token=…", text: $agent.serverURL)
                        .textFieldStyle(.roundedBorder)
                    HStack {
                        Button(agent.connected ? "Reconnect" : "Connect") { agent.connect() }
                        Button("Disconnect") { agent.disconnect() }.disabled(!agent.connected)
                    }
                }
                .padding(.top, Theme.spaceXs)
            }
        }
    }

    private var setupBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceSm) {
            connectBody
            Divider()
            permissionsBody
        }
        .padding(.top, Theme.spaceXs)
    }

    private var permissionsBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceSm) {
            Text("Permissions").font(.caption).foregroundStyle(.secondary)
            permissionRow(
                label: "Accessibility",
                granted: agent.axGranted,
                grant: { Permissions.promptAccessibility(); Permissions.openAccessibilitySettings() })
            permissionRow(
                label: "Screen Recording",
                granted: agent.screenGranted,
                grant: { Permissions.requestScreenRecording(); Permissions.openScreenRecordingSettings() })
            Button("Refresh") { agent.refreshPermissions() }.font(.caption)
            Text("After granting, quit and relaunch so the app re-reads its trust status.")
                .font(.caption2).foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private func permissionRow(label: String, granted: Bool, grant: @escaping () -> Void) -> some View {
        HStack {
            Image(systemName: granted ? "checkmark.circle.fill" : "exclamationmark.circle")
                .foregroundStyle(granted ? Theme.success : Theme.warning)
            Text(label)
            Spacer()
            if !granted { Button("Grant", action: grant).font(.caption) }
        }
    }

    private static let clock: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()
}
