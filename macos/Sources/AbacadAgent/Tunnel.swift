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

    private func frame(_ type: UInt8, _ id: UInt64, _ payload: Data) -> Data {
        var out = Data([type])
        var be = id.bigEndian
        withUnsafeBytes(of: &be) { out.append(contentsOf: $0) }
        out.append(payload)
        return out
    }
}
