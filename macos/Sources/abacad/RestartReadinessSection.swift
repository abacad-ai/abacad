import Foundation
import SwiftUI

struct RestartReadinessSection: View {
    @ObservedObject var agent: Agent
    @StateObject private var monitor = RestartReadinessMonitor()

    var body: some View {
        DisclosureGroup {
            readinessBody
        } label: {
            readinessHeader
        }
        .font(.caption)
        .onAppear {
            agent.refreshLaunchAtLogin()
            monitor.refresh()
        }
    }

    private enum Visual {
        case ready
        case action
        case recommended
        case changing
        case unknown

        var icon: String {
            switch self {
            case .ready: return "checkmark.circle.fill"
            case .action: return "exclamationmark.circle.fill"
            case .recommended: return "info.circle.fill"
            case .changing: return "arrow.triangle.2.circlepath.circle"
            case .unknown: return "questionmark.circle"
            }
        }

        var color: Color {
            switch self {
            case .ready: return Theme.success
            case .action, .changing: return Theme.warning
            case .recommended: return Theme.inkMuted
            case .unknown: return Theme.inkSubtle
            }
        }
    }

    private typealias Presentation = (
        visual: Visual,
        label: String,
        guidance: String?
    )

    private var automaticLoginMatchesCurrentUser: Bool? {
        switch monitor.snapshot.automaticLogin {
        case .enabled(let user):
            guard let user else { return nil }
            return user.caseInsensitiveCompare(monitor.snapshot.currentUser) == .orderedSame
        case .disabled:
            return false
        case .unknown:
            return nil
        }
    }

    private var unattendedBootReady: Bool {
        monitor.snapshot.fileVault == .disabled &&
            automaticLoginMatchesCurrentUser == true &&
            agent.launchAtLoginStatus == .enabled
    }

    private var unattendedServerReady: Bool {
        unattendedBootReady &&
            monitor.snapshot.passwordAfterSleep == .disabled &&
            monitor.snapshot.systemSleep == .disabled &&
            monitor.snapshot.restartAfterPowerFailure == .enabled
    }

    private var summary: (title: String, icon: String, color: Color) {
        if monitor.checking {
            return ("Checking", "arrow.triangle.2.circlepath", Theme.inkMuted)
        }
        if unattendedServerReady {
            return ("Ready", "checkmark.circle.fill", Theme.success)
        }
        if monitor.snapshot.fileVault == .enabled {
            return ("Login required", "lock.fill", Theme.warning)
        }
        let criticalUnknown =
            monitor.snapshot.fileVault == .unknown ||
            monitor.snapshot.fileVault == .changing ||
            automaticLoginMatchesCurrentUser == nil
        if criticalUnknown {
            return ("Unknown", "questionmark.circle", Theme.inkSubtle)
        }
        return ("Check settings", "exclamationmark.circle.fill", Theme.warning)
    }

    private var readinessHeader: some View {
        let state = summary
        return HStack(spacing: Theme.spaceXs) {
            Text("Availability after restart")
            Spacer()
            Image(systemName: state.icon)
                .foregroundStyle(state.color)
                .accessibilityHidden(true)
            Text(state.title)
                .font(.caption2)
                .foregroundStyle(state.color)
        }
    }

    private var readinessBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceSm) {
            VStack(alignment: .leading, spacing: Theme.spaceXs) {
                modeDescription(
                    icon: "lock.shield",
                    title: "Secure mode",
                    detail: "Keep FileVault on. Someone must unlock and sign in after every restart.")
                modeDescription(
                    icon: "desktopcomputer",
                    title: "Unattended server mode",
                    detail: "Turn FileVault off and enable automatic login so this Mac returns without anyone present.")
                Text("Unattended mode removes boot-time protection. Anyone with physical access can enter this account.")
                    .font(.caption2)
                    .foregroundStyle(Theme.warning)
            }

            Divider()
            Text("Unattended server checklist")
                .font(.caption)
                .foregroundStyle(.secondary)

            launchAtLoginBody
            readinessRow(title: "FileVault", presentation: fileVaultPresentation)
            readinessRow(title: "Automatic login", presentation: automaticLoginPresentation)
            readinessRow(
                title: "Password after idle",
                presentation: settingPresentation(
                    monitor.snapshot.passwordAfterSleep,
                    readyWhen: .disabled,
                    readyLabel: "Never",
                    actionLabel: "Required",
                    guidance: "System Settings → Lock Screen → Require password after screen saver or display off → Never"))
            readinessRow(
                title: "System sleep",
                presentation: settingPresentation(
                    monitor.snapshot.systemSleep,
                    readyWhen: .disabled,
                    readyLabel: "Prevented",
                    actionLabel: "Allowed",
                    guidance: "System Settings → Energy → Prevent automatic sleeping when the display is off"))
            readinessRow(
                title: "Power-loss startup",
                presentation: settingPresentation(
                    monitor.snapshot.restartAfterPowerFailure,
                    readyWhen: .enabled,
                    readyLabel: "Enabled",
                    actionLabel: "Disabled",
                    guidance: "System Settings → Energy → Start up automatically after a power failure"))
            readinessRow(title: "Wake for network access", presentation: wakeForNetworkPresentation)

            HStack {
                Button {
                    agent.refreshLaunchAtLogin()
                    monitor.refresh()
                } label: {
                    Label(monitor.checking ? "Checking" : "Recheck",
                          systemImage: "arrow.clockwise")
                }
                .disabled(monitor.checking)
                Spacer()
                Text("Read-only checks")
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
            }
        }
        .padding(.top, Theme.spaceXs)
    }

    private func modeDescription(icon: String, title: String, detail: String) -> some View {
        HStack(alignment: .top, spacing: Theme.spaceSm) {
            Image(systemName: icon)
                .frame(width: 16)
                .foregroundStyle(.secondary)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 1) {
                Text(title).font(.caption)
                Text(detail).font(.caption2).foregroundStyle(.secondary)
            }
        }
    }

    private var fileVaultPresentation: Presentation {
        switch monitor.snapshot.fileVault {
        case .disabled:
            return (.ready, "Off", nil)
        case .enabled:
            return (.action, "On",
                    "System Settings → Privacy & Security → FileVault → Turn Off")
        case .changing:
            return (.changing, "Changing",
                    "Wait for FileVault encryption or decryption to finish, then recheck.")
        case .unknown:
            return (.unknown, "Unknown",
                    "Open System Settings → Privacy & Security → FileVault to confirm.")
        }
    }

    private var automaticLoginPresentation: Presentation {
        switch monitor.snapshot.automaticLogin {
        case .enabled(let user):
            guard let user else {
                return (.unknown, "Enabled",
                        "Automatic login is on, but the selected account could not be confirmed.")
            }
            if user.caseInsensitiveCompare(monitor.snapshot.currentUser) == .orderedSame {
                return (.ready, user, nil)
            }
            return (.action, user,
                    "System Settings → Users & Groups → Automatically log in as \(monitor.snapshot.currentUser)")
        case .disabled:
            return (.action, "Disabled",
                    "System Settings → Users & Groups → Automatically log in as \(monitor.snapshot.currentUser)")
        case .unknown:
            return (.unknown, "Unknown",
                    "Open System Settings → Users & Groups to confirm automatic login.")
        }
    }

    private var wakeForNetworkPresentation: Presentation {
        switch monitor.snapshot.wakeForNetwork {
        case .enabled:
            return (.ready, "Enabled", nil)
        case .disabled:
            return (.recommended, "Off",
                    "Recommended: System Settings → Energy → Wake for network access")
        case .changing:
            return (.changing, "Changing", nil)
        case .unknown:
            return (.unknown, "Unknown",
                    "Check Wake for network access under System Settings → Energy.")
        }
    }

    private func settingPresentation(
        _ state: SystemSettingState,
        readyWhen expected: SystemSettingState,
        readyLabel: String,
        actionLabel: String,
        guidance: String
    ) -> Presentation {
        if state == expected { return (.ready, readyLabel, nil) }
        switch state {
        case .changing:
            return (.changing, "Changing", "Wait for the setting change to finish, then recheck.")
        case .unknown:
            return (.unknown, "Unknown", guidance)
        default:
            return (.action, actionLabel, guidance)
        }
    }

    private func readinessRow(title: String, presentation: Presentation) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Image(systemName: presentation.visual.icon)
                    .frame(width: 14)
                    .foregroundStyle(presentation.visual.color)
                    .accessibilityHidden(true)
                Text(title)
                Spacer(minLength: Theme.spaceXs)
                Text(presentation.label)
                    .font(.caption2)
                    .foregroundStyle(presentation.visual.color)
                    .multilineTextAlignment(.trailing)
            }
            if let guidance = presentation.guidance {
                Text(guidance)
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .padding(.leading, 20)
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var launchAtLoginBody: some View {
        VStack(alignment: .leading, spacing: Theme.spaceXs) {
            Toggle("Start at login", isOn: Binding(
                get: { agent.launchAtLoginEnabled },
                set: { agent.setLaunchAtLogin($0) }
            ))
            .toggleStyle(.switch)
            .controlSize(.small)

            if agent.launchAtLoginNeedsApproval {
                Text("Approval is required in System Settings before abacad can start automatically.")
                    .font(.caption2)
                    .foregroundStyle(Theme.warning)
                Button("Open Login Items") { LaunchAtLogin.openSettings() }
                    .font(.caption)
            } else if agent.launchAtLoginUnavailable {
                Text("Move abacad to Applications, then enable this again.")
                    .font(.caption2)
                    .foregroundStyle(Theme.warning)
            } else if let error = agent.launchAtLoginError {
                Text(error).font(.caption2).foregroundStyle(.red)
            }
        }
    }
}
