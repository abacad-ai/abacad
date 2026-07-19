import Foundation

// Outbound WebSocket to the abacad relay's /device endpoint. The Mac dials out
// (NAT-friendly; the server never connects in). Text frames carry the JSON
// command/reply lane; binary frames carry the tunnel lane. Auto-reconnects with
// exponential backoff, and pings to keep the idle socket alive.
final class WebSocketClient: NSObject, URLSessionWebSocketDelegate, @unchecked Sendable {
    var onText: ((String) -> Void)?
    var onBinary: ((Data) -> Void)?
    var onStateChange: ((Bool) -> Void)?

    private var session: URLSession!
    private var task: URLSessionWebSocketTask?
    private let sendQueue = DispatchQueue(label: "abacad.ws.send")
    private var url: URL?
    private var closedByUser = false
    private var backoff: TimeInterval = 1
    private(set) var connected = false {
        didSet { if connected != oldValue { onStateChange?(connected) } }
    }

    override init() {
        super.init()
        let cfg = URLSessionConfiguration.default
        cfg.waitsForConnectivity = true
        session = URLSession(configuration: cfg, delegate: self, delegateQueue: nil)
    }

    func connect(urlString: String) {
        // URL(string:) accepts non-ws schemes (e.g. a bare "host:port" parses with
        // "host" as the scheme), but webSocketTask(with:) throws an uncaught
        // NSException for anything that isn't ws/wss. Since connect() runs during
        // Agent.init(), that throw would kill the process before the menu-bar item
        // is installed — so validate the scheme here and refuse a bad URL instead.
        guard let u = URL(string: urlString),
              let scheme = u.scheme?.lowercased(),
              scheme == "ws" || scheme == "wss" else { return }
        closedByUser = false
        url = u
        openSocket()
    }

    func disconnect() {
        closedByUser = true
        task?.cancel(with: .goingAway, reason: nil)
        task = nil
        connected = false
    }

    private func openSocket() {
        guard let u = url, !closedByUser else { return }
        let t = session.webSocketTask(with: u)
        // Relay screenshots are multi-MB base64; lift the receive cap generously.
        t.maximumMessageSize = 32 * 1024 * 1024
        task = t
        t.resume()
        receiveLoop()
        schedulePing()
    }

    private func receiveLoop() {
        task?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                switch message {
                case .string(let s): self.onText?(s)
                case .data(let d): self.onBinary?(d)
                @unknown default: break
                }
                self.receiveLoop() // re-arm for the next frame
            case .failure:
                self.handleDrop()
            }
        }
    }

    private func handleDrop() {
        guard !closedByUser else { return }
        connected = false
        task = nil
        let delay = backoff
        backoff = min(backoff * 2, 15) // cap at 15s, matching the Android client
        sendQueue.asyncAfter(deadline: .now() + delay) { [weak self] in self?.openSocket() }
    }

    private func schedulePing() {
        sendQueue.asyncAfter(deadline: .now() + 20) { [weak self] in
            guard let self, let t = self.task else { return }
            t.sendPing { _ in }
            self.schedulePing()
        }
    }

    func send(text: String) {
        sendQueue.async { [weak self] in self?.task?.send(.string(text)) { _ in } }
    }

    func send(data: Data) {
        sendQueue.async { [weak self] in self?.task?.send(.data(data)) { _ in } }
    }

    // MARK: URLSessionWebSocketDelegate
    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask,
                    didOpenWithProtocol proto: String?) {
        backoff = 1
        connected = true
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask,
                    didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        handleDrop()
    }
}
