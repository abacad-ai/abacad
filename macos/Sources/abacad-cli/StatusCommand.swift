import AbacadKit
import Foundation

// `abacad status` — what is configured on this Mac, without connecting to
// anything. Answers the two questions someone actually has after running
// `abacad connect` over ssh: did the pairing stick, and is there anything here
// that can act on it?
enum StatusCommand {
    private static let appPath = "/Applications/abacad.app"

    static func run() -> Int32 {
        Capabilities.load()

        let dialURL = Prefs.serverURL
        let relay = Prefs.relayURL
        let deviceID = Prefs.deviceID
        let hasToken = !Prefs.deviceToken.isEmpty

        print("abacad \(Enrollment.appVersion())")
        print("")

        if !dialURL.isEmpty {
            print("Paired:      yes (\(redactToken(dialURL)))")
        } else if !deviceID.isEmpty && hasToken {
            print("Paired:      self-enrolled as \(deviceID)")
            print("Relay:       \(relay.isEmpty ? Enrollment.defaultRelay : relay)")
        } else {
            print("Paired:      no — run `abacad connect`")
        }

        let exposed = Capabilities.report()
        if exposed == [Capabilities.wildcard] {
            print("Exposes:     everything (not configured — `abacad capabilities` to narrow)")
        } else if exposed.isEmpty {
            print("Exposes:     nothing")
        } else {
            print("Exposes:     \(exposed.count) of \(Capabilities.all.count) — \(exposed.joined(separator: ", "))")
        }

        // The distinction that matters on macOS: this binary configures, the app
        // connects. Pairing with no app installed is a legitimate intermediate
        // state, not an error — but silence about it would be misleading.
        if FileManager.default.fileExists(atPath: appPath) {
            print("App:         installed at \(appPath)")
        } else {
            print("App:         not installed")
            print("")
            print("Nothing on this Mac will answer an agent until the app is running —")
            print("this CLI configures, the app connects. Install it from")
            print("\(ConnectFlow.defaultServer)/downloads.")
        }
        return 0
    }

    /// The dial URL carries the device token in its query. Never print it: this
    /// output goes into terminal scrollback, tickets and screenshots.
    private static func redactToken(_ url: String) -> String {
        guard var parts = URLComponents(string: url), let items = parts.queryItems else { return url }
        parts.queryItems = items.map {
            $0.name == "token" ? URLQueryItem(name: "token", value: "…") : $0
        }
        return parts.string ?? url
    }
}
