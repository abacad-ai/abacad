import Foundation
import Security

// Persisted settings. The server URL carries the device token (?token=…), so it
// is stored in the login Keychain rather than UserDefaults — a plist that any
// process running as the user, or a Time Machine backup, can read. A value left
// in UserDefaults by an older build is migrated into the Keychain on first read.
public enum Prefs {
    private static let service = "ai.abacad.agent"
    private static let legacyDefaultsKey = "server_url"

    /// The full dial URL (wss://…?token=…). Written by `abacad connect` and by a
    /// human pasting into the panel. Self-enrollment leaves this empty and uses
    /// relayURL + deviceToken instead.
    public static var serverURL: String {
        get {
            if let v = keychainGet("server_url") { return v }
            // Migrate a value written by an older UserDefaults-backed build.
            let legacy = UserDefaults.standard.string(forKey: legacyDefaultsKey) ?? ""
            if !legacy.isEmpty {
                keychainSet("server_url", legacy)
                UserDefaults.standard.removeObject(forKey: legacyDefaultsKey)
                return legacy
            }
            return ""
        }
        set { keychainSet("server_url", newValue) }
    }

    // Self-enrollment credentials. The token is a bearer secret, so it lives in
    // the same login Keychain as serverURL rather than UserDefaults.
    public static var relayURL: String {
        get { keychainGet("relay_url") ?? "" }
        set { keychainSet("relay_url", newValue) }
    }

    public static var deviceID: String {
        get { keychainGet("device_id") ?? "" }
        set { keychainSet("device_id", newValue) }
    }

    /// The device-side capability ceiling, comma-separated (see Capabilities).
    /// Not a secret — it is a policy decision about this machine — but it lives
    /// with the rest so there is one store to reason about. Empty means never
    /// configured, which is the wildcard; Capabilities writes an explicit marker
    /// for the empty set, because keychainGet cannot tell "" from a missing item.
    public static var capabilities: String {
        get { keychainGet("capabilities") ?? "" }
        set { keychainSet("capabilities", newValue) }
    }

    public static var deviceToken: String {
        get { keychainGet("device_token") ?? "" }
        set { keychainSet("device_token", newValue) }
    }

    /// Drop the self-enrollment credential, keeping the relay so a re-register
    /// goes back to the same server. Used when the relay stops recognizing us.
    public static func clearEnrollment() {
        deviceID = ""
        deviceToken = ""
    }

    private static func baseQuery(_ account: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }

    private static func keychainGet(_ account: String) -> String? {
        var query = baseQuery(account)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data,
              let s = String(data: data, encoding: .utf8), !s.isEmpty else { return nil }
        return s
    }

    private static func keychainSet(_ account: String, _ value: String) {
        let data = Data(value.utf8)
        // Upsert: update in place, or add if the item doesn't exist yet.
        let status = SecItemUpdate(baseQuery(account) as CFDictionary,
                                   [kSecValueData as String: data] as CFDictionary)
        if status == errSecItemNotFound {
            var add = baseQuery(account)
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            SecItemAdd(add as CFDictionary, nil)
        }
    }
}
