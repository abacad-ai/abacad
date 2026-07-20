import Foundation
import Network

// The binary tunnel lane, mirroring server/mock-desktop.mjs and the framing in
// internal/protocol/stream.go. The server opens a stream (StreamOpen with a
// "host:port"); this dials that TCP target and pipes bytes both ways. Frames:
//   [type:1][stream id:8 BE][payload]   type 1=Open 2=Data 3=Close
// Streams are only ever opened from the server side; the device just answers.
final class Tunnel {
    private static let OPEN: UInt8 = 1
    private static let DATA: UInt8 = 2
    private static let CLOSE: UInt8 = 3

    /// Sends a binary frame back over the WebSocket. Set by the owner.
    var sendFrame: ((Data) -> Void)?

    private let queue = DispatchQueue(label: "abacad.tunnel")
    private var conns: [UInt64: NWConnection] = [:]

    /// Handle one inbound binary frame from the server.
    func handle(_ frame: Data) {
        let bytes = [UInt8](frame)
        guard bytes.count >= 9 else { return }
        let type = bytes[0]
        var id: UInt64 = 0
        for i in 0..<8 { id = (id << 8) | UInt64(bytes[1 + i]) }
        let payload = Data(bytes[9...])
        switch type {
        case Self.OPEN: open(id, target: String(data: payload, encoding: .utf8) ?? "")
        case Self.DATA: queue.async { self.conns[id]?.send(content: payload, completion: .contentProcessed { _ in }) }
        case Self.CLOSE: queue.async { self.conns.removeValue(forKey: id)?.cancel() }
        default: break
        }
    }

    private func open(_ id: UInt64, target: String) {
        guard let colon = target.lastIndex(of: ":"),
              let port = NWEndpoint.Port(String(target[target.index(after: colon)...])) else {
            emitClose(id, reason: "bad target \(target)")
            return
        }
        let host = String(target[target.startIndex..<colon])
        // Refuse targets with no legitimate tunnel use and clear SSRF value: the
        // cloud metadata endpoint (169.254.169.254) and other link-local /
        // unspecified / multicast addresses. Loopback and private ranges stay
        // allowed — reaching this Mac's own services and LAN is the point. The
        // server enforces the same policy; this is device-side defense in depth.
        if Self.isBlockedTargetHost(host) {
            emitClose(id, reason: "target \(host) is not an allowed address")
            return
        }
        let conn = NWConnection(host: NWEndpoint.Host(host), port: port, using: .tcp)
        queue.async { self.conns[id] = conn }
        conn.stateUpdateHandler = { [weak self] state in
            switch state {
            case .failed(let err): self?.emitClose(id, reason: "\(err)")
            case .cancelled: break
            default: break
            }
        }
        receive(id, conn)
        conn.start(queue: queue)
    }

    private func receive(_ id: UInt64, _ conn: NWConnection) {
        conn.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, error in
            guard let self else { return }
            if let d = data, !d.isEmpty { self.sendFrame?(self.frame(Self.DATA, id, d)) }
            if isComplete || error != nil {
                self.emitClose(id, reason: error.map { "\($0)" } ?? "")
                return
            }
            self.receive(id, conn)
        }
    }

    // Tear down locally and tell the server the stream closed (empty reason = EOF).
    private func emitClose(_ id: UInt64, reason: String) {
        queue.async {
            guard let conn = self.conns.removeValue(forKey: id) else { return }
            conn.cancel()
            self.sendFrame?(self.frame(Self.CLOSE, id, Data(reason.utf8)))
        }
    }

    func closeAll() {
        queue.async {
            for c in self.conns.values { c.cancel() }
            self.conns.removeAll()
        }
    }

    /// Best-effort SSRF guard: block link-local (incl. 169.254.169.254 metadata),
    /// unspecified, and multicast literals. Numeric range checks apply only to
    /// real IPv4 literals so a hostname like "224.example.com" isn't flagged.
    /// Loopback and private ranges are intentionally allowed.
    static func isBlockedTargetHost(_ host: String) -> Bool {
        let h = host.lowercased()
        // IPv6: unspecified, link-local (fe80::/10), multicast (ff00::/8).
        if h == "::" || h.hasPrefix("fe80:") || (h.hasPrefix("ff") && h.contains(":")) {
            return true
        }
        // IPv4: only judge genuine dotted-quad literals.
        let parts = h.split(separator: ".")
        if parts.count == 4 {
            var octets: [Int] = []
            for p in parts {
                guard let n = Int(p), n >= 0, n <= 255 else { return false }
                octets.append(n)
            }
            if octets == [0, 0, 0, 0] { return true }              // unspecified
            if octets[0] == 169 && octets[1] == 254 { return true } // link-local incl. metadata
            if octets[0] >= 224 && octets[0] <= 239 { return true } // multicast
        }
        return false
    }

    private func frame(_ type: UInt8, _ id: UInt64, _ payload: Data) -> Data {
        var out = Data([type])
        var be = id.bigEndian
        withUnsafeBytes(of: &be) { out.append(contentsOf: $0) }
        out.append(payload)
        return out
    }
}
