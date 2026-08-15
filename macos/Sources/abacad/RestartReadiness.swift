import Combine
import Foundation

enum SystemSettingState: Equatable, Sendable {
    case enabled
    case disabled
    case changing
    case unknown
}

enum AutomaticLoginState: Equatable, Sendable {
    case enabled(user: String?)
    case disabled
    case unknown
}

struct RestartReadinessSnapshot: Equatable, Sendable {
    let currentUser: String
    let fileVault: SystemSettingState
    let automaticLogin: AutomaticLoginState
    let passwordAfterSleep: SystemSettingState
    let systemSleep: SystemSettingState
    let restartAfterPowerFailure: SystemSettingState
    let wakeForNetwork: SystemSettingState

    static func unknown(currentUser: String = NSUserName()) -> RestartReadinessSnapshot {
        RestartReadinessSnapshot(
            currentUser: currentUser,
            fileVault: .unknown,
            automaticLogin: .unknown,
            passwordAfterSleep: .unknown,
            systemSleep: .unknown,
            restartAfterPowerFailure: .unknown,
            wakeForNetwork: .unknown)
    }
}

// Read-only posture monitor for the settings that decide whether a remote Mac
// returns after restart, idle, and power loss. The command output is not a
// protocol, so every parser fails closed to .unknown when Apple changes it.
final class RestartReadinessMonitor: ObservableObject {
    @Published private(set) var snapshot = RestartReadinessSnapshot.unknown()
    @Published private(set) var checking = false

    private var generation = 0

    func refresh() {
        generation &+= 1
        let currentGeneration = generation
        let currentUser = NSUserName()
        checking = true

        DispatchQueue.global(qos: .utility).async {
            let result = RestartReadinessProbe.inspect(currentUser: currentUser)
            DispatchQueue.main.async { [weak self] in
                guard let self, self.generation == currentGeneration else { return }
                self.snapshot = result
                self.checking = false
            }
        }
    }
}

private enum RestartReadinessProbe {
    static func inspect(currentUser: String) -> RestartReadinessSnapshot {
        let power = powerSettings()
        return RestartReadinessSnapshot(
            currentUser: currentUser,
            fileVault: fileVaultStatus(),
            automaticLogin: automaticLoginStatus(),
            passwordAfterSleep: screenLockStatus(),
            systemSleep: sleepState(power),
            restartAfterPowerFailure: state(for: power?["autorestart"]),
            wakeForNetwork: state(for: power?["womp"]))
    }

    private static func fileVaultStatus() -> SystemSettingState {
        guard let result = run("/usr/bin/fdesetup", ["status"]),
              result.status == 0 else {
            return .unknown
        }
        let output = result.text.lowercased()
        if output.contains("decryption in progress") ||
            output.contains("encryption in progress") ||
            output.contains("deferred enablement") {
            return .changing
        }
        if output.contains("filevault is on") { return .enabled }
        if output.contains("filevault is off") { return .disabled }
        return .unknown
    }

    private static func automaticLoginStatus() -> AutomaticLoginState {
        guard let result = run("/usr/sbin/sysadminctl", ["-autologin", "status"]),
              result.status == 0 else { return .unknown }
        let status = result.text.lowercased()
        if status.contains("automatic login is off") ||
            status.contains("automatic login is disabled") ||
            status.contains("autologin is off") {
            return .disabled
        }

        let reportedUser = value(
            after: "automatic login user:",
            in: result.text)
        if let reportedUser {
            return .enabled(user: reportedUser)
        }

        if status.contains("automatic login is on") ||
            status.contains("automatic login is enabled") ||
            status.contains("autologin is on") {
            return .enabled(user: automaticLoginUser())
        }
        return .unknown
    }

    private static func automaticLoginUser() -> String? {
        guard let output = run(
            "/usr/bin/defaults",
            ["read", "/Library/Preferences/com.apple.loginwindow", "autoLoginUser"]),
            output.status == 0 else { return nil }
        let trimSet = CharacterSet.whitespacesAndNewlines
            .union(CharacterSet(charactersIn: "\""))
        let value = output.text.trimmingCharacters(in: trimSet)
        return value.isEmpty ? nil : value
    }

    private static func screenLockStatus() -> SystemSettingState {
        guard let result = run(
            "/usr/sbin/sysadminctl", ["-screenLock", "status"]),
              result.status == 0 else { return .unknown }
        let output = result.text.lowercased()
        if output.contains("screenlock is off") ||
            output.contains("screen lock is off") ||
            output.contains("screenlock is disabled") {
            return .disabled
        }
        if output.contains("screenlock delay is immediate") ||
            numericValue(after: "screenlock delay is ", in: output) != nil {
            return .enabled
        }
        if output.contains("screenlock is on") ||
            output.contains("screen lock is on") ||
            output.contains("screenlock is enabled") {
            return .enabled
        }
        return .unknown
    }

    private static func powerSettings() -> [String: Int]? {
        guard let output = run("/usr/bin/pmset", ["-g", "custom"]),
              output.status == 0 else { return nil }
        var ac: [String: Int] = [:]
        var inACPower = false
        var sawPowerHeader = false

        for rawLine in output.text.split(whereSeparator: \.isNewline) {
            let line = String(rawLine).trimmingCharacters(in: .whitespaces)
            let lower = line.lowercased()
            if lower.hasSuffix("power:") {
                sawPowerHeader = true
                inACPower = lower.contains("ac power")
                continue
            }
            guard inACPower || !sawPowerHeader else { continue }
            let fields = line.split(whereSeparator: { $0 == " " || $0 == "\t" })
            guard fields.count >= 2, let value = Int(fields.last!) else { continue }
            ac[String(fields[0]).lowercased()] = value
        }
        return ac.isEmpty ? nil : ac
    }

    private static func value(after marker: String, in text: String) -> String? {
        for rawLine in text.split(whereSeparator: \.isNewline) {
            let line = String(rawLine)
            guard let range = line.range(of: marker, options: .caseInsensitive) else {
                continue
            }
            let value = line[range.upperBound...]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            if !value.isEmpty { return value }
        }
        return nil
    }

    private static func numericValue(after marker: String, in text: String) -> Double? {
        guard let value = value(after: marker, in: text),
              let token = value.split(whereSeparator: \.isWhitespace).first
        else { return nil }
        return Double(token)
    }

    private static func sleepState(_ settings: [String: Int]?) -> SystemSettingState {
        if settings?["disablesleep"] == 1 { return .disabled }
        return state(for: settings?["sleep"])
    }

    private static func state(for value: Int?) -> SystemSettingState {
        guard let value else { return .unknown }
        return value == 0 ? .disabled : .enabled
    }

    private struct CommandOutput {
        let status: Int32
        let text: String
    }

    private static func run(
        _ executable: String,
        _ arguments: [String],
        timeout: TimeInterval = 4
    ) -> CommandOutput? {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: executable)
        process.arguments = arguments
        var environment = ProcessInfo.processInfo.environment
        environment["LANG"] = "C"
        environment["LC_ALL"] = "C"
        process.environment = environment

        let stdout = Pipe()
        let stderr = Pipe()
        process.standardOutput = stdout
        process.standardError = stderr

        let finished = DispatchSemaphore(value: 0)
        process.terminationHandler = { _ in finished.signal() }
        do {
            try process.run()
        } catch {
            return nil
        }
        if finished.wait(timeout: .now() + timeout) == .timedOut {
            process.terminate()
            _ = finished.wait(timeout: .now() + 1)
            return nil
        }

        let out = stdout.fileHandleForReading.readDataToEndOfFile()
        let err = stderr.fileHandleForReading.readDataToEndOfFile()
        let text = [out, err]
            .compactMap { String(data: $0, encoding: .utf8) }
            .filter { !$0.isEmpty }
            .joined(separator: "\n")
        return CommandOutput(status: process.terminationStatus, text: text)
    }
}
