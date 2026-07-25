import Foundation

// Outbound WebSocket to the abacad relay's /device endpoint. The Mac dials out
// (NAT-friendly; the server never connects in). Text frames carry the JSON
// command/reply lane; binary frames carry the tunnel lane. Auto-reconnects with
// exponential backoff, and pings to keep the idle socket alive.
//
// Exactly one socket at a time. URLSession reports a drop twice — the pending
// receive fails AND the delegate's didCloseWith fires — so every callback is
// tagged with the generation of the socket it belongs to, and a report from a
// superseded socket is ignored. Without that guard one drop scheduled two
// reconnects, each opening a socket the other didn't know about; the relay then
// evicted the losers (Hub.Register) and the device's activity log filled with
// connect/disconnect churn. All mutable state lives on sendQueue for the same
// reason: it is touched from the delegate queue, the receive completion, and the
// ping timer.
final class WebSocketClient: NSObject, URLSessionWebSocketDelegate, @unchecked Sendable {
    var onText: ((String) -> Void)?
    var onBinary: ((Data) -> Void)?
    var onStateChange: ((Bool) -> Void)?

    private var session: URLSession!
    private let sendQueue = DispatchQueue(label: "abacad.ws.send")

    // A dial that never opens and never fails: waitsForConnectivity parks a task
    // indefinitely with no network, and a stalled TLS/upgrade behaves the same.
    // Since forceReconnect deliberately leaves an in-flight dial alone, bound it.
    private let dialTimeout: TimeInterval = 45

    // Liveness, mirroring what the relay does to us from the other side. A ping
    // that errors is a dead socket, but the worse case is a ping that is simply
    // never answered — on a black-holed path (the classic post-wake one) the
    // completion handler never fires at all, so without a deadline the client
    // sits "connected" to nothing, and every wake trigger politely believes it.
    // Each probe gets pongTimeout to come back; pongMissBudget consecutive
    // misses is a drop.
    private let pingInterval: TimeInterval = 20
    private let pongTimeout: TimeInterval = 10
    private let pongMissBudget = 3

    // --- sendQueue-owned state ---------------------------------------------
    private var task: URLSessionWebSocketTask?
    private var gen = 0                         // bumped per socket; stale callbacks no-op
    private var url: URL?
    private var authToken: String?
    private var closedByUser = false
    private var backoff: TimeInterval = 1
    private var pendingRetry: DispatchWorkItem?  // cancellable, so a wake can dial now
    private var dialing = false                  // a socket is opening but not yet up
    private var dialWatchdog: DispatchWorkItem?
    private var pongMisses = 0                   // consecutive unanswered probes
    private var pongDeadline: DispatchWorkItem?  // at most one probe is ever outstanding
    private var connected = false {
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
        guard var comps = URLComponents(string: urlString),
              let scheme = comps.scheme?.lowercased(),
              scheme == "ws" || scheme == "wss" else { return }
        // Refuse a plaintext control channel to anything but loopback: this Mac
        // carries screen contents + input injection, so a cleartext hop is a full
        // MITM. Real servers must use wss://.
        let host = comps.host ?? ""
        if scheme == "ws" && !Self.isLoopback(host) { return }
        // Carry the device token in the Authorization header, not the URL, so it
        // stays out of logs and proxy access logs. Strip any ?token= from a stored
        // URL and migrate it to the header.
        let token = comps.queryItems?.first(where: { $0.name == "token" })?.value
        comps.queryItems = comps.queryItems?.filter { $0.name != "token" }
        // Advertise our version so the relay can show it in the dashboard /
        // list_devices. Unlike the token it stays in the URL — the server reads
        // ?version= off the dial. CFBundleShortVersionString is stamped from the
        // repo-root VERSION at package time (see the macos Makefile target).
        if comps.queryItems?.contains(where: { $0.name == "version" }) != true {
            let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "dev"
            comps.queryItems = (comps.queryItems ?? []) + [URLQueryItem(name: "version", value: version)]
        }
        if comps.queryItems?.isEmpty == true { comps.queryItems = nil }
        guard let u = comps.url else { return }
        sendQueue.async { [weak self] in
            guard let self else { return }
            self.authToken = token
            self.closedByUser = false
            self.url = u
            self.backoff = 1
            self.openSocket()
        }
    }

    private static func isLoopback(_ host: String) -> Bool {
        host == "127.0.0.1" || host == "::1" || host == "localhost"
    }

    func disconnect() {
        sendQueue.async { [weak self] in
            guard let self else { return }
            self.closedByUser = true
            self.pendingRetry?.cancel()
            self.pendingRetry = nil
            self.dialWatchdog?.cancel()
            self.dialWatchdog = nil
            self.dialing = false
            self.pongDeadline?.cancel()
            self.pongDeadline = nil
            self.pongMisses = 0
            self.gen &+= 1 // orphan every in-flight callback
            self.task?.cancel(with: .goingAway, reason: nil)
            self.task = nil
            self.connected = false
        }
    }

    /// Bring the socket back *now* if it isn't up — called on wake from sleep and
    /// when the network path returns, so a socket that died while the Mac was
    /// asleep doesn't wait out the backoff (or, worse, wait for URLSession to
    /// notice a connection that has been dead for hours).
    ///
    /// "Up" is a belief, not a fact: right after a wake the socket may be dead
    /// while `connected` still says true, because nothing has reported the drop
    /// yet. So when we think we're connected we probe instead of trusting it — an
    /// unanswered ping is the earliest evidence the socket died silently (address
    /// change, NAT rebind). A dial already in flight is left alone; restarting it
    /// on every path update is how you manufacture the connection bursts this
    /// whole change exists to stop. Mirrors DeviceClient.forceReconnect().
    func forceReconnect() {
        sendQueue.async { [weak self] in
            guard let self, !self.closedByUser, self.url != nil else { return }
            if self.connected, let t = self.task {
                // A deliberate liveness check, so it gets no second chance: if this
                // one probe isn't answered the socket is gone and we redial.
                self.probe(t, gen: self.gen, budget: 1)
                return
            }
            if self.dialing { return }
            self.backoff = 1
            self.openSocket()
        }
    }

    // Opens a fresh socket, superseding whatever came before. sendQueue only.
    private func openSocket() {
        guard let u = url, !closedByUser else { return }
        pendingRetry?.cancel()
        pendingRetry = nil
        // Cancel the socket we're replacing rather than just dropping the
        // reference: an abandoned task stays open, and the relay would keep it in
        // the hub until a newer connection evicted it.
        gen &+= 1
        let g = gen
        task?.cancel(with: .goingAway, reason: nil)
        connected = false
        dialing = true
        pongDeadline?.cancel()
        pongDeadline = nil
        pongMisses = 0

        var req = URLRequest(url: u)
        if let token = authToken {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        let t = session.webSocketTask(with: req)
        // Relay screenshots are multi-MB base64; lift the receive cap generously.
        t.maximumMessageSize = 32 * 1024 * 1024
        task = t
        t.resume()
        receiveLoop(task: t, gen: g)
        schedulePing(gen: g)
        armDialWatchdog(gen: g)
    }

    // A dial that neither opens nor fails would otherwise sit forever, with
    // forceReconnect politely declining to disturb it. Give up after dialTimeout
    // and let the normal backoff retry.
    private func armDialWatchdog(gen g: Int) {
        dialWatchdog?.cancel()
        let work = DispatchWorkItem { [weak self] in
            guard let self, self.gen == g, self.dialing else { return }
            self.dialWatchdog = nil
            self.handleDrop(gen: g)
        }
        dialWatchdog = work
        sendQueue.asyncAfter(deadline: .now() + dialTimeout, execute: work)
    }

    private func receiveLoop(task t: URLSessionWebSocketTask, gen g: Int) {
        t.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                switch message {
                case .string(let s): self.onText?(s)
                case .data(let d): self.onBinary?(d)
                @unknown default: break
                }
                self.sendQueue.async {
                    guard self.gen == g else { return } // superseded socket
                    self.receiveLoop(task: t, gen: g)   // re-arm for the next frame
                }
            case .failure:
                self.handleDrop(gen: g)
            }
        }
    }

    // One drop → one reconnect. Reports from a socket we've already replaced (and
    // the second report for the same socket, since URLSession delivers both a
    // failed receive and didCloseWith) fall out on the generation check.
    private func handleDrop(gen g: Int) {
        sendQueue.async { [weak self] in
            guard let self, self.gen == g, !self.closedByUser else { return }
            self.gen &+= 1
            self.dialWatchdog?.cancel()
            self.dialWatchdog = nil
            self.dialing = false
            self.pongDeadline?.cancel()
            self.pongDeadline = nil
            self.pongMisses = 0
            self.task?.cancel(with: .goingAway, reason: nil)
            self.task = nil
            self.connected = false
            let delay = self.backoff
            self.backoff = min(self.backoff * 2, 15) // cap at 15s, matching the Android client
            let work = DispatchWorkItem { [weak self] in
                guard let self else { return }
                self.pendingRetry = nil
                self.openSocket()
            }
            self.pendingRetry = work
            self.sendQueue.asyncAfter(deadline: .now() + delay, execute: work)
        }
    }

    // Keeps the idle socket (and any NAT binding in front of it) alive, and is our
    // only detector of a socket that died without an error — the pending receive
    // can stay blocked on a connection the network dropped hours ago. Bound to its
    // socket's generation so a reconnect doesn't leave a second chain running.
    private func schedulePing(gen g: Int) {
        sendQueue.asyncAfter(deadline: .now() + pingInterval) { [weak self] in
            guard let self, self.gen == g, let t = self.task else { return }
            self.probe(t, gen: g, budget: self.pongMissBudget)
            self.schedulePing(gen: g)
        }
    }

    // Sends one ping and requires a pong within pongTimeout. An error means the
    // socket is already gone; silence past the deadline counts as a miss, and
    // `budget` consecutive misses is a drop. Any answer resets the count.
    // sendQueue only.
    private func probe(_ t: URLSessionWebSocketTask, gen g: Int, budget: Int) {
        pongDeadline?.cancel()
        let deadline = DispatchWorkItem { [weak self] in
            guard let self, self.gen == g else { return }
            self.pongDeadline = nil
            self.pongMisses += 1
            if self.pongMisses >= budget { self.handleDrop(gen: g) }
        }
        pongDeadline = deadline
        sendQueue.asyncAfter(deadline: .now() + pongTimeout, execute: deadline)
        t.sendPing { [weak self] err in
            guard let self else { return }
            self.sendQueue.async {
                guard self.gen == g else { return } // superseded socket
                deadline.cancel()
                if err != nil {
                    self.handleDrop(gen: g)
                } else {
                    self.pongMisses = 0
                }
            }
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
        sendQueue.async { [weak self] in
            guard let self, webSocketTask === self.task else { return } // stale socket
            self.dialWatchdog?.cancel()
            self.dialWatchdog = nil
            self.dialing = false
            self.backoff = 1
            self.connected = true
        }
    }

    func urlSession(_ session: URLSession, webSocketTask: URLSessionWebSocketTask,
                    didCloseWith closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        sendQueue.async { [weak self] in
            guard let self, webSocketTask === self.task else { return } // stale socket
            self.handleDrop(gen: self.gen)
        }
    }
}
