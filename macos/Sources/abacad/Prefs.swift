import Foundation

// Persisted settings. Only the server URL (which carries the ?token=…) is stored,
// mirroring the Android client where the whole ws URL including the token is kept
// verbatim in SharedPreferences.
enum Prefs {
    private static let serverURLKey = "server_url"

    static var serverURL: String {
        get { UserDefaults.standard.string(forKey: serverURLKey) ?? "" }
        set { UserDefaults.standard.set(newValue, forKey: serverURLKey) }
    }
}
