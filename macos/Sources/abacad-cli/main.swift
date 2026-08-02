import AbacadKit
import Foundation

// `abacad` — the macOS command line.
//
// This configures the Mac; it does not drive it. Screen capture and input
// injection stay in the menu-bar app, because macOS keys Screen Recording and
// Accessibility grants to a bundle identity (ai.abacad.mac) — a bare binary run
// from a terminal inherits the *terminal's* TCC identity instead, so a CLI agent
// would either not work or, worse, quietly attach those grants to Terminal.app.
// The Linux CLI is a full headless daemon precisely because X11 has no such
// rule; here the honest split is CLI configures, app runs.
//
// Both halves read and write the same login-Keychain store (service
// ai.abacad.agent, no access group), so pairing from the terminal and pairing
// from the panel are the same act. Pairing before the app is installed is a fine
// order of operations — the app picks the credential up on first launch.
//
//   abacad connect [--server URL]     pair this Mac with a workspace
//   abacad capabilities [flags]       what this Mac is willing to expose
//   abacad status                     what is configured right now

setvbuf(stdout, nil, _IONBF, 0)

let argv = Array(CommandLine.arguments.dropFirst())

switch argv.first {
case "connect":
    exit(ConnectFlow.run(argv))
case "capabilities":
    exit(CapabilitiesCommand.run(Array(argv.dropFirst())))
case "status":
    exit(StatusCommand.run())
case "--version", "-v", "version":
    print(Enrollment.appVersion())
    exit(0)
case "help", "--help", "-h", nil:
    usage()
    exit(argv.first == nil ? 1 : 0)
default:
    FileHandle.standardError.write(Data("abacad: unknown command \"\(argv[0])\"\n\n".utf8))
    usage()
    exit(1)
}

func usage() {
    print("""
        usage: abacad <command> [flags]

          connect [--server URL]   pair this Mac with a workspace
          capabilities [flags]     show or change what this Mac exposes
          status                   show what is configured
          version                  print the version

        This is the configuration CLI. The menu-bar app is what actually connects
        and answers an agent — install it from \(ConnectFlow.defaultServer)/downloads.
        """)
}
