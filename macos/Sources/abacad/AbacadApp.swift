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
        tunnel.sendFrame = { [weak self] data in self?.ws.send(data: data) }
        ws.onStateChange = { [weak self] up in
            guard let self else { return }
            // A fresh socket starts out assumed active server-side, so re-report
            // our real power state on every (re)connect.
            if up { self.sendPresence(self.asleep ? "asleep" : "active") }
            DispatchQueue.main.async {
                self.connected = up
                self.event(up ? "• connected" : "• disconnected")
            }
        }
        ws.onText = { [weak self] text in self?.handle(text: text) }
        ws.onBinary = { [weak self] data in self?.tunnel.handle(data) }
        observeSystemEvents()
        if !serverURL.isEmpty { ws.connect(urlString: serverURL) }
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

    func connect() {
        let url = serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
        Prefs.serverURL = url
        serverURL = url
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

    private var ready: Bool { agent.connected && agent.axGranted && agent.screenGranted }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.spaceMd) {
            stateHeader

            if agent.watched || agent.recording { awarenessFlags }

            if ready {
                Divider()
                recentActions
                Divider()
                DisclosureGroup("Setup & connection") { setupBody }
                    .font(.caption)
            } else {
                Divider()
                connectBody
                Divider()
                permissionsBody
            }

            Divider()
            Button("Quit abacad") { NSApplication.shared.terminate(nil) }
        }
        .padding(Theme.spaceLg)
        .frame(width: 340)
        .onAppear { agent.refreshPermissions() }
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

    private var connectBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceXs) {
            Text("Server URL").font(.caption).foregroundStyle(.secondary)
            TextField("wss://host/device?token=…", text: $agent.serverURL)
                .textFieldStyle(.roundedBorder)
            HStack {
                Button(agent.connected ? "Reconnect" : "Connect") { agent.connect() }
                Button("Disconnect") { agent.disconnect() }.disabled(!agent.connected)
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
