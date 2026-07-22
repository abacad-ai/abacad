import Foundation
import CryptoKit

// Device side of the /blobs data plane, backing the push_file / pull_file verbs.
// File bytes ride HTTP — not the command WebSocket — so a large file never has to
// be base64'd onto a text frame. Authenticated with the same per-device token the
// socket uses, carried in the Authorization header.
//
// Streamed end to end: download writes the response straight to a temp file (then
// moves it), upload streams from the file handle. Neither buffers the whole object.
struct BlobClient {
    let base: String       // e.g. https://host/blobs
    let token: String?

    /// Derive the /blobs endpoint from the relay URL: same host, over http(s)
    /// instead of ws(s). Returns nil if the URL isn't a parseable ws/wss URL,
    /// which disables file transfer rather than pointing it somewhere wrong.
    static func fromServerURL(_ raw: String) -> BlobClient? {
        guard var comps = URLComponents(string: raw.trimmingCharacters(in: .whitespacesAndNewlines)),
              let scheme = comps.scheme?.lowercased() else { return nil }
        switch scheme {
        case "wss": comps.scheme = "https"
        case "ws": comps.scheme = "http"
        default: return nil
        }
        let token = comps.queryItems?.first(where: { $0.name == "token" })?.value
        comps.query = nil
        comps.fragment = nil
        comps.path = "/blobs"
        guard let url = comps.url else { return nil }
        return BlobClient(base: url.absoluteString, token: token)
    }

    private func authed(_ req: inout URLRequest) {
        if let t = token { req.setValue("Bearer \(t)", forHTTPHeaderField: "Authorization") }
    }

    /// Stream the blob to destPath and return (bytesWritten, hexSha256). Downloads
    /// to a temp file and moves it into place, so a reader never sees a partial
    /// file. The parent directory must already exist.
    func download(blobID: String, destPath: String, mode: Int) async throws -> (Int64, String) {
        guard let url = URL(string: base + "/" + blobID) else { throw CmdError.message("bad blob URL") }
        var req = URLRequest(url: url)
        authed(&req)
        let (tmpURL, resp) = try await URLSession.shared.download(for: req)
        guard let http = resp as? HTTPURLResponse, http.statusCode == 200 else {
            try? FileManager.default.removeItem(at: tmpURL)
            throw CmdError.message("blob download failed: HTTP \((resp as? HTTPURLResponse)?.statusCode ?? -1)")
        }
        let (size, sha) = try Self.hashFile(tmpURL)
        let dest = URL(fileURLWithPath: destPath)
        let fm = FileManager.default
        if fm.fileExists(atPath: dest.path) { try fm.removeItem(at: dest) }
        try fm.moveItem(at: tmpURL, to: dest)
        try fm.setAttributes([.posixPermissions: mode], ofItemAtPath: dest.path)
        return (size, sha)
    }

    /// Stream srcPath to /blobs and return (blobID, size, hexSha256).
    func upload(srcPath: String) async throws -> (String, Int64, String) {
        let src = URL(fileURLWithPath: srcPath)
        var isDir: ObjCBool = false
        guard FileManager.default.fileExists(atPath: src.path, isDirectory: &isDir) else {
            throw CmdError.message("no such file: \(srcPath)")
        }
        if isDir.boolValue { throw CmdError.message("\(srcPath) is a directory, not a file") }
        guard let url = URL(string: base) else { throw CmdError.message("bad blob URL") }
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")
        authed(&req)
        let (data, resp) = try await URLSession.shared.upload(for: req, fromFile: src)
        guard let http = resp as? HTTPURLResponse, http.statusCode == 201 else {
            throw CmdError.message("blob upload failed: HTTP \((resp as? HTTPURLResponse)?.statusCode ?? -1)")
        }
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let id = obj["id"] as? String else {
            throw CmdError.message("bad blob upload response")
        }
        let size: Int64 = (obj["size"] as? Int).map(Int64.init)
            ?? (obj["size"] as? Double).map { Int64($0) } ?? 0
        let sha = obj["sha256"] as? String ?? ""
        return (id, size, sha)
    }

    /// Stream a file through SHA-256, returning (size, hexDigest) without loading
    /// the whole file into memory.
    private static func hashFile(_ url: URL) throws -> (Int64, String) {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var hasher = SHA256()
        var total: Int64 = 0
        while let chunk = try handle.read(upToCount: 1 << 20), !chunk.isEmpty {
            hasher.update(data: chunk)
            total += Int64(chunk.count)
        }
        let hex = hasher.finalize().map { String(format: "%02x", $0) }.joined()
        return (total, hex)
    }
}
