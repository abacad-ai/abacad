import Foundation
import Security

// Persisted settings. The server URL carries the device token (?token=…), so it
// is stored in the login Keychain rather than UserDefaults — a plist that any
// process running as the user, or a Time Machine backup, can read. A value left
// in UserDefaults by an older build is migrated into the Keychain on first read.
enum Prefs {
    private static let service = "ai.abacad.agent"
    private static let account = "server_url"
    private static let legacyDefaultsKey = "server_url"

    static var serverURL: String {
        get {
            if let v = keychainGet() { return v }
            // Migrate a value written by an older UserDefaults-backed build.
            let legacy = UserDefaults.standard.string(forKey: legacyDefaultsKey) ?? ""
            if !legacy.isEmpty {
                keychainSet(legacy)
                UserDefaults.standard.removeObject(forKey: legacyDefaultsKey)
                return legacy
            }
            return ""
        }
        set { keychainSet(newValue) }
    }

    private static func baseQuery() -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
    }

    private static func keychainGet() -> String? {
        var query = baseQuery()
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: AnyObject?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return nil }
        return String(data: data, encoding: .utf8)
    }

    private static func keychainSet(_ value: String) {
        let data = Data(value.utf8)
        // Upsert: update in place, or add if the item doesn't exist yet.
        let status = SecItemUpdate(baseQuery() as CFDictionary,
                                   [kSecValueData as String: data] as CFDictionary)
        if status == errSecItemNotFound {
            var add = baseQuery()
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            SecItemAdd(add as CFDictionary, nil)
        }
    }
}
