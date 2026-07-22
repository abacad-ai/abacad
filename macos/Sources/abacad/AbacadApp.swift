import SwiftUI
import AppKit

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
            // connection state is exposed via the accessibility label.
            Image(nsImage: RelayMark.trayImage())
                .accessibilityLabel(agent.connected ? "abacad — connected" : "abacad — disconnected")
        }
        .menuBarExtraStyle(.window)
    }
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

    private let ws = WebSocketClient()
    private var dispatcher = CommandDispatcher()
    private let tunnel = Tunnel()

    init() {
        dispatcher.blobClient = BlobClient.fromServerURL(serverURL)
        tunnel.sendFrame = { [weak self] data in self?.ws.send(data: data) }
        ws.onStateChange = { [weak self] up in
            DispatchQueue.main.async { self?.connected = up }
        }
        ws.onText = { [weak self] text in self?.handle(text: text) }
        ws.onBinary = { [weak self] data in self?.tunnel.handle(data) }
        if !serverURL.isEmpty { ws.connect(urlString: serverURL) }
    }

    func connect() {
        let url = serverURL.trimmingCharacters(in: .whitespacesAndNewlines)
        Prefs.serverURL = url
        serverURL = url
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

    // Parse a command frame and dispatch it; reply is correlated by id.
    private func handle(text: String) {
        guard let cmd = Json.object(text) else { return } // malformed → no reply
        let id = cmd["id"] as? String ?? ""
        let method = cmd.string("method")
        let params = cmd["params"] as? [String: Any] ?? [:]
        Task.detached { [ws = ws, dispatcher = dispatcher] in
            do {
                let result = try await dispatcher.execute(method: method, params: params)
                ws.send(text: Json.string(["id": id, "ok": true, "result": result]))
            } catch let CmdError.message(m) {
                ws.send(text: Json.string(["id": id, "ok": false, "error": m]))
            } catch {
                ws.send(text: Json.string(["id": id, "ok": false, "error": "\(error)"]))
            }
        }
    }
}

// The menu-bar panel: connection, server URL, permission grants.
struct AgentPanel: View {
    @ObservedObject var agent: Agent

    var body: some View {
        // Panel chrome stays native (materials, system font); colors and spacing
        // come from Theme so status reads identically to the dashboard and the
        // Android probe.
        VStack(alignment: .leading, spacing: Theme.spaceMd) {
            HStack {
                Circle().fill(agent.connected ? Theme.success : Theme.inkSubtle)
                    .frame(width: 8, height: 8)
                Text(agent.connected ? "Connected" : "Disconnected").font(.headline)
            }

            VStack(alignment: .leading, spacing: Theme.spaceXs) {
                Text("Server URL").font(.caption).foregroundStyle(.secondary)
                TextField("wss://host:8848/device?token=…", text: $agent.serverURL)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 320)
                HStack {
                    Button(agent.connected ? "Reconnect" : "Connect") { agent.connect() }
                    Button("Disconnect") { agent.disconnect() }.disabled(!agent.connected)
                }
            }

            Divider()

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
                    .font(.caption2).foregroundStyle(.secondary).frame(width: 320, alignment: .leading)
            }

            Divider()
            Button("Quit abacad") { NSApplication.shared.terminate(nil) }
        }
        .padding(Theme.spaceLg)
        .onAppear { agent.refreshPermissions() }
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
        .frame(width: 320)
    }
}
