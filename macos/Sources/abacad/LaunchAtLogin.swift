import Foundation
import ServiceManagement

// The menu-bar app is the agent on macOS: Screen Recording and Accessibility
// grants belong to this bundle, so a separate headless daemon would not inherit
// them. Register the main app itself and let launchd start it in the user's
// graphical session after sign-in.
enum LaunchAtLogin {
    private static let preferenceKey = "launchAtLoginPreferenceSet"

    static var status: SMAppService.Status {
        SMAppService.mainApp.status
    }

    static func isRequested(_ status: SMAppService.Status) -> Bool {
        status == .enabled || status == .requiresApproval
    }

    static func setEnabled(_ enabled: Bool) throws {
        try apply(enabled)
        // Remember both choices. Without this, a user who turns startup off
        // would have it silently restored the next time the configured app
        // launches and runs the one-time migration below.
        UserDefaults.standard.set(enabled, forKey: preferenceKey)
    }

    // Existing installs predate the toggle, while new installs may be paired by
    // the CLI before the app ever opens. A stored device credential proves that
    // setup was explicit, so default those configured agents to run at login
    // exactly once. A later opt-out is preserved by preferenceKey.
    static func enableAfterSetupIfUnconfigured() throws {
        guard UserDefaults.standard.object(forKey: preferenceKey) == nil else { return }
        try apply(true)
        UserDefaults.standard.set(true, forKey: preferenceKey)
    }

    private static func apply(_ enabled: Bool) throws {
        let service = SMAppService.mainApp
        if enabled {
            guard !isRequested(service.status) else { return }
            try service.register()
        } else {
            guard service.status != .notRegistered && service.status != .notFound else { return }
            try service.unregister()
        }
    }

    static func openSettings() {
        SMAppService.openSystemSettingsLoginItems()
    }
}
