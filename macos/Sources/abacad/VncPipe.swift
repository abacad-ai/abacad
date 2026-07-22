import Foundation
import Network

// macOS live channel (screen_recording live): the client never implements RFB. It
// pipes the dedicated reverse-connect WebSocket to the system's real VNC server —
// macOS built-in Screen Sharing on 127.0.0.1:5900 — exactly as the Linux client
// pipes to x11vnc. RFB (handshake, encodings, dirty-rects) is Screen Sharing's job.
//
// Setup (one-time, by the user): System Settings → General → Sharing → enable
// Screen Sharing, and under its options enable "VNC viewers may control screen with
// password" + set a password (so noVNC can authenticate with VNC auth). No admin
// needed at runtime; nothing GPL ships in this app.
actor VncPipe {
    static let shared = VncPipe()
    private var bridge: VncBridge?

    func handle(params: [String: Any]) async throws -> [String: Any] {
        switch params.string("action") {
        case "start":
            try start(url: params.string("url"))
            return ["started": true]
        case "stop":
            stop()
            return ["stopped": true]
        default:
            throw CmdError.message(#"vnc action must be "start" or "stop""#)
        }
    }

    private func start(url urlStr: String) throws {
        guard let url = URL(string: urlStr) else { throw CmdError.message("vnc start requires a valid url") }
        stop()
        let b = VncBridge(url: url, tcpHost: "127.0.0.1", tcpPort: 5900)
        b.onEnd = { [weak self] in Task { await self?.clear(b) } }
        bridge = b
        b.start()
    }

    func stop() {
        bridge?.end()
        bridge = nil
    }

    private func clear(_ b: VncBridge) {
        if bridge === b { bridge = nil }
    }
}

// VncBridge relays bytes between an outbound WebSocket (URLSessionWebSocketTask) and
// a localhost TCP connection (NWConnection) to the system VNC server, both ways,
// until either side ends. @unchecked Sendable: the two pumps touch distinct
// endpoints and end() is idempotent.
final class VncBridge: @unchecked Sendable {
    private let ws: URLSessionWebSocketTask
    private let tcp: NWConnection
    private var ended = false
    var onEnd: (() -> Void)?

    init(url: URL, tcpHost: String, tcpPort: UInt16) {
        ws = URLSession(configuration: .default).webSocketTask(with: url)
        tcp = NWConnection(host: NWEndpoint.Host(tcpHost),
                           port: NWEndpoint.Port(rawValue: tcpPort)!, using: .tcp)
    }

    func start() {
        ws.resume()
        tcp.stateUpdateHandler = { [weak self] state in
            switch state {
            case .ready:
                self?.pumpTCPToWS()
                self?.pumpWSToTCP()
            case .failed, .cancelled:
                self?.end()
            default:
                break
            }
        }
        tcp.start(queue: .global(qos: .userInitiated))
    }

    private func pumpWSToTCP() {
        ws.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                let data: Data
                switch message {
                case .data(let d): data = d
                case .string(let s): data = Data(s.utf8)
                @unknown default: data = Data()
                }
                if !data.isEmpty {
                    self.tcp.send(content: data, completion: .contentProcessed { _ in })
                }
                self.pumpWSToTCP()
            case .failure:
                self.end()
            }
        }
    }

    private func pumpTCPToWS() {
        tcp.receive(minimumIncompleteLength: 1, maximumLength: 64 << 10) { [weak self] data, _, isComplete, error in
            guard let self else { return }
            if let data, !data.isEmpty {
                self.ws.send(.data(data)) { _ in }
            }
            if error != nil || isComplete {
                self.end()
                return
            }
            self.pumpTCPToWS()
        }
    }

    func end() {
        if ended { return }
        ended = true
        ws.cancel(with: .goingAway, reason: nil)
        tcp.cancel()
        let cb = onEnd
        onEnd = nil
        cb?()
    }
}
