import Foundation

// The macOS live channel (screen_recording live): a minimal, view-only RFB (VNC)
// server spoken directly over the dedicated reverse-connect WebSocket. On "start"
// it dials the server's VNC ingress and serves RFB to whatever noVNC viewer the
// server bridges in: the RFB banner + security(None) + ServerInit, then a
// framebuffer update (Raw-encoded BGRX pixels from ScreenCapture) per client
// request. Input messages are parsed and dropped — view only for now (matches the
// v0 live.mode default). The pixels ride this dedicated WS, never the command
// socket.
//
// UNVERIFIED at runtime: the RFB byte protocol (pixel-format handling in
// particular) needs a real noVNC client to shake out.
actor VNCHandler {
    static let shared = VNCHandler()

    private var task: Task<Void, Never>?
    private var ws: URLSessionWebSocketTask?

    func handle(params: [String: Any]) async throws -> [String: Any] {
        switch params.string("action") {
        case "start":
            try await start(url: params.string("url"))
            return ["started": true]
        case "stop":
            stop()
            return ["stopped": true]
        default:
            throw CmdError.message(#"vnc action must be "start" or "stop""#)
        }
    }

    private func start(url urlStr: String) async throws {
        guard let url = URL(string: urlStr) else { throw CmdError.message("vnc start requires a valid url") }
        stop()
        let session = URLSession(configuration: .default)
        let socket = session.webSocketTask(with: url)
        socket.resume()
        ws = socket
        let stream = WSStream(socket)
        task = Task { [weak self] in
            do {
                try await VNCHandler.serve(stream)
            } catch {
                // Connection closed or protocol ended; drop the session.
            }
            await self?.stop()
        }
    }

    func stop() {
        task?.cancel()
        task = nil
        ws?.cancel(with: .goingAway, reason: nil)
        ws = nil
    }

    // MARK: RFB (view-only, Raw encoding)

    private static func serve(_ s: WSStream) async throws {
        // Handshake (RFB 3.8).
        try await s.write(Array("RFB 003.008\n".utf8)) // ProtocolVersion
        _ = try await s.read(12) // client version
        try await s.write([1, 1]) // 1 security type: None(1)
        _ = try await s.read(1) // client selects
        try await s.write([0, 0, 0, 0]) // SecurityResult: OK
        _ = try await s.read(1) // ClientInit (shared flag)

        var frame = try await ScreenCapture.captureRawBGRA()
        try await s.write(serverInit(w: frame.w, h: frame.h))

        while !Task.isCancelled {
            let type = try await s.read(1)[0]
            switch type {
            case 0: // SetPixelFormat (view-only: ignore, keep BGRX)
                _ = try await s.read(19)
            case 2: // SetEncodings
                let hdr = try await s.read(3)
                let count = Int(hdr[1]) << 8 | Int(hdr[2])
                if count > 0 { _ = try await s.read(count * 4) }
            case 3: // FramebufferUpdateRequest
                _ = try await s.read(9)
                frame = try await ScreenCapture.captureRawBGRA()
                try await s.write(framebufferUpdate(frame))
            case 4: // KeyEvent
                _ = try await s.read(7)
            case 5: // PointerEvent
                _ = try await s.read(5)
            case 6: // ClientCutText
                let hdr = try await s.read(7)
                let n = Int(hdr[3]) << 24 | Int(hdr[4]) << 16 | Int(hdr[5]) << 8 | Int(hdr[6])
                if n > 0 { _ = try await s.read(n) }
            default:
                throw CmdError.message("unknown RFB client message \(type)")
            }
        }
    }

    private static func serverInit(w: Int, h: Int) -> [UInt8] {
        var b = [UInt8]()
        b += be16(w)
        b += be16(h)
        // PIXEL_FORMAT: 32bpp, depth 24, little-endian, true-colour, BGRX
        // (redShift 16, greenShift 8, blueShift 0).
        b += [32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0]
        let name = Array("abacad".utf8)
        b += be32(name.count)
        b += name
        return b
    }

    private static func framebufferUpdate(_ f: (w: Int, h: Int, pixels: [UInt8])) -> [UInt8] {
        var b = [UInt8]()
        b += [0, 0]        // message type 0, padding
        b += be16(1)       // one rectangle
        b += be16(0)       // x
        b += be16(0)       // y
        b += be16(f.w)     // width
        b += be16(f.h)     // height
        b += be32(0)       // encoding 0 = Raw
        b += f.pixels      // w*h*4 BGRX
        return b
    }

    private static func be16(_ v: Int) -> [UInt8] { [UInt8((v >> 8) & 0xff), UInt8(v & 0xff)] }
    private static func be32(_ v: Int) -> [UInt8] {
        [UInt8((v >> 24) & 0xff), UInt8((v >> 16) & 0xff), UInt8((v >> 8) & 0xff), UInt8(v & 0xff)]
    }
}

// WSStream turns a URLSessionWebSocketTask into a byte stream with blocking
// "read exactly n" reads (accumulating across frames) and binary writes — the
// natural shape for driving a request/response byte protocol like RFB.
private final class WSStream {
    private let task: URLSessionWebSocketTask
    private var buffer = [UInt8]()

    init(_ task: URLSessionWebSocketTask) { self.task = task }

    func read(_ n: Int) async throws -> [UInt8] {
        while buffer.count < n {
            switch try await task.receive() {
            case .data(let d): buffer.append(contentsOf: d)
            case .string(let s): buffer.append(contentsOf: Array(s.utf8))
            @unknown default: break
            }
        }
        let out = Array(buffer.prefix(n))
        buffer.removeFirst(n)
        return out
    }

    func write(_ bytes: [UInt8]) async throws {
        try await task.send(.data(Data(bytes)))
    }
}
