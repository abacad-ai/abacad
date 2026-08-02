import AbacadKit
import Foundation

// `abacad capabilities` — the device-side control over what this Mac exposes.
//
//   abacad capabilities                       # show what is on and off
//   abacad capabilities --disable files,ssh   # turn groups or names off
//   abacad capabilities --enable screenshot   # turn them back on
//   abacad capabilities --only screenshot     # exactly this, nothing else
//   abacad capabilities --none                # expose nothing
//   abacad capabilities --all                 # expose everything (the default)
//
// The menu-bar panel has had these switches all along; until now the terminal had
// no equivalent, so a Mac you administer over ssh could be paired but not
// constrained. The flag vocabulary and the group names are deliberately identical
// to the Linux client's `abacad capabilities`, so the same command means the same
// thing on either machine.
//
// Changes are written to the login Keychain and take effect the next time the app
// starts — a running app read the ceiling at launch and is a separate process
// from this one.
enum CapabilitiesCommand {
    // The same groupings the dashboard shows, so someone moving between the two
    // is choosing the same things by the same names.
    static let groups: [String: [String]] = [
        "observe": ["screenshot", "screen_recording"],
        "control": [
            "tap", "long_press", "swipe", "input_text",
            "back", "home", "recents",
            "click", "right_click", "drag", "scroll", "press_keys", "composite",
        ],
        "files": ["push_file", "pull_file"],
        "execute": ["execute"],
        "live": ["vnc"],
        "tunnel": ["tunnel"],
        "ssh": ["ssh"],
    ]

    static func run(_ args: [String]) -> Int32 {
        var enable = "", disable = "", only = ""
        var all = false, none = false

        var i = 0
        while i < args.count {
            let a = args[i]
            let next = i + 1 < args.count ? args[i + 1] : nil
            switch a {
            case "--all": all = true
            case "--none": none = true
            case "--enable", "--disable", "--only":
                guard let v = next else {
                    return fail("\(a) needs a value")
                }
                if a == "--enable" { enable = v } else if a == "--disable" { disable = v } else { only = v }
                i += 1
            case "--help", "-h":
                usage()
                return 0
            default:
                if let eq = a.range(of: "="), a.hasPrefix("--") {
                    let key = String(a[a.startIndex..<eq.lowerBound])
                    let val = String(a[eq.upperBound...])
                    switch key {
                    case "--enable": enable = val
                    case "--disable": disable = val
                    case "--only": only = val
                    default: return fail("unknown flag \(key)")
                    }
                } else {
                    return fail("unexpected argument \"\(a)\"")
                }
            }
            i += 1
        }
        if all && none { return fail("--all and --none contradict each other") }

        Capabilities.load()

        var changed = false
        do {
            if all {
                Capabilities.set([Capabilities.wildcard])
                changed = true
            } else if none {
                Capabilities.set([])
                changed = true
            } else if !only.isEmpty {
                Capabilities.set(try expand(only))
                changed = true
            }
            for (list, on) in [(enable, true), (disable, false)] where !list.isEmpty {
                try expand(list).forEach { Capabilities.toggle($0, on: on) }
                changed = true
            }
        } catch CmdError.message(let m) {
            return fail(m)
        } catch {
            return fail("\(error)")
        }

        printStatus(changed: changed)
        return 0
    }

    /// Resolves a comma-separated list of group and capability names, failing on
    /// anything unrecognized rather than silently ignoring it — a typo that
    /// quietly did nothing would leave someone believing they had turned
    /// something off when they had not.
    private static func expand(_ list: String) throws -> [String] {
        var out: [String] = []
        for raw in list.split(separator: ",") {
            let name = raw.trimmingCharacters(in: .whitespaces)
            if name.isEmpty { continue }
            if let members = groups[name] {
                out.append(contentsOf: members)
            } else if Capabilities.all.contains(name) {
                out.append(name)
            } else {
                throw CmdError.message(
                    "unknown capability or group \"\(name)\" (groups: \(groupNames().joined(separator: ", ")))")
            }
        }
        return out
    }

    private static func groupNames() -> [String] { groups.keys.sorted() }

    private static func printStatus(changed: Bool) {
        let on = Set(Capabilities.enabledList())
        print("This Mac exposes:")
        for c in Capabilities.all {
            print("  \(on.contains(c) ? " on" : "off")  \(c)")
        }
        if changed {
            print("\nSaved. Restart the abacad app for this to take effect,")
            print("and it will be reported to the relay on reconnect.")
        }
        // Say the sharp part out loud, in the place someone is actually making the
        // decision. A tunnel can dial this machine's own sshd, so "ssh off, tunnel
        // on" is not the closed door it looks like.
        if on.contains("tunnel") && !on.contains("ssh") {
            print("\nNote: ssh is off but tunnel is on — a tunnel can dial this machine's")
            print("own port 22 directly, so ssh is still reachable. Disable tunnel too.")
        }
    }

    private static func usage() {
        print("""
            usage: abacad capabilities [--enable X] [--disable X] [--only X] [--all] [--none]

            Groups: \(groupNames().joined(separator: ", "))
            Names:  \(Capabilities.all.joined(separator: ", "))
            """)
    }

    private static func fail(_ message: String) -> Int32 {
        FileHandle.standardError.write(Data("abacad: \(message)\n".utf8))
        return 1
    }
}
