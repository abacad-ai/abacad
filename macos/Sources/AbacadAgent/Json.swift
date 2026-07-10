import Foundation

// Tiny JSON helpers. The wire uses dynamic `params` objects, so we work in
// [String: Any] via JSONSerialization rather than Codable — it keeps the command
// envelope and per-method params handling uniform and lax (matching the Android
// client, which silently drops malformed frames).

enum Json {
    /// Parse a text frame into a top-level object. Returns nil on any error.
    static func object(_ text: String) -> [String: Any]? {
        guard let data = text.data(using: .utf8),
              let obj = try? JSONSerialization.jsonObject(with: data),
              let dict = obj as? [String: Any] else { return nil }
        return dict
    }

    /// Serialize an object to a compact JSON string. Returns "{}" if it can't.
    static func string(_ obj: [String: Any]) -> String {
        guard let data = try? JSONSerialization.data(withJSONObject: obj),
              let s = String(data: data, encoding: .utf8) else { return "{}" }
        return s
    }
}

// Convenience typed getters over a loose params dict.
extension Dictionary where Key == String, Value == Any {
    func int(_ k: String, _ def: Int = 0) -> Int {
        if let n = self[k] as? Int { return n }
        if let d = self[k] as? Double { return Int(d) }
        if let s = self[k] as? String, let n = Int(s) { return n }
        return def
    }
    func double(_ k: String, _ def: Double = 0) -> Double {
        if let d = self[k] as? Double { return d }
        if let n = self[k] as? Int { return Double(n) }
        return def
    }
    func string(_ k: String, _ def: String = "") -> String { self[k] as? String ?? def }
    func bool(_ k: String, _ def: Bool = false) -> Bool { self[k] as? Bool ?? def }
    func strings(_ k: String) -> [String] { (self[k] as? [Any])?.compactMap { $0 as? String } ?? [] }
    func objects(_ k: String) -> [[String: Any]] { (self[k] as? [Any])?.compactMap { $0 as? [String: Any] } ?? [] }
}

/// A method handler either succeeds with a result object or fails with a message.
enum CmdError: Error { case message(String) }
