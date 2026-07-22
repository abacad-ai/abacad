import Foundation

// `abacad connect` — the device-authorization grant (RFC 8628) that enrolls this
// Mac without pasting a token into the menu-bar panel. It asks the server for a
// short code, prints the URL to approve it, polls until the human approves in
// their browser, then stores the issued wss://…?token=… URL via Prefs (login
// Keychain) so the menu-bar app auto-connects on next launch. The CLI peer of the
// Linux and Windows `abacad connect`.
enum ConnectFlow {
    static let defaultServer = "https://abacad.ai"

    /// Entry from the process launcher. Returns a process exit code.
    static func run(_ args: [String]) -> Int32 {
        // Stream progress even when stdout is piped/redirected: the URL + code
        // must appear before we block polling, but stdout is block-buffered off a
        // TTY, which would hide them until exit.
        setvbuf(stdout, nil, _IONBF, 0)

        var server = defaultServer
        var i = 1 // args[0] is "connect"
        while i < args.count {
            let a = args[i]
            if a == "--server" || a == "-s", i + 1 < args.count {
                server = args[i + 1]
                i += 2
                continue
            }
            if a.hasPrefix("--server=") {
                server = String(a.dropFirst("--server=".count))
            }
            i += 1
        }
        server = server.trimmingCharacters(in: .whitespaces)
        while server.hasSuffix("/") { server.removeLast() }
        if server.isEmpty {
            printErr("empty --server")
            return 1
        }

        // Bridge the async flow to this synchronous entry: a class holder keeps the
        // result Sendable-safe across the Task boundary.
        final class Box { var code: Int32 = 1 }
        let box = Box()
        let done = DispatchSemaphore(value: 0)
        Task {
            box.code = await runAsync(server: server)
            done.signal()
        }
        done.wait()
        return box.code
    }

    private static func runAsync(server: String) async -> Int32 {
        // 1. Start the pairing, reporting our platform so the approval page shows it.
        guard let s = await post(server + "/api/devices/pair/start", body: ["platform": "macos"]) else {
            printErr("start pairing: could not reach \(server)")
            return 1
        }
        guard s.status == 201 || s.status == 200,
              let start = s.json, !start.string("device_code").isEmpty
        else {
            printErr("start pairing: " + serverError(s.json, s.status))
            return 1
        }
        let deviceCode = start.string("device_code")
        let userCode = start.string("user_code")
        var link = start.string("verification_uri_complete")
        if link.isEmpty { link = start.string("verification_uri") }
        let interval = max(start.int("interval"), 1)
        let expiresIn = max(start.int("expires_in"), 60)

        print("")
        print("To connect this device, open:")
        print("")
        print("    \(link)")
        print("")
        print("and approve it while signed in (code: \(userCode)). Waiting…")

        // 2. Poll until the human approves, honoring the interval + lifetime hints.
        let deadline = Date().addingTimeInterval(TimeInterval(expiresIn))
        while true {
            if Date() > deadline {
                printErr("timed out waiting for approval")
                return 1
            }
            guard let p = await post(server + "/api/devices/pair/poll", body: ["device_code": deviceCode]) else {
                printErr("poll: could not reach \(server)")
                return 1
            }
            switch p.status {
            case 200:
                let wss = p.json?.string("wss_url") ?? ""
                let token = p.json?.string("device_token") ?? ""
                if wss.isEmpty {
                    printErr("approved but server returned no wss_url")
                    return 1
                }
                // The server's wss_url already carries ?token=; store it whole (the
                // format Prefs/WebSocketClient expect), re-attaching only if absent.
                Prefs.serverURL = withToken(wss, token)
                print("")
                print("✓ Approved. Credentials saved to the login Keychain.")
                print("  Launch abacad (the menu-bar app) to go online — it auto-connects.")
                return 0
            case 202: // still pending
                try? await Task.sleep(nanoseconds: UInt64(interval) * 1_000_000_000)
            default: // 403 denied / 404 unknown / 410 expired-or-used → terminal
                printErr(serverError(p.json, p.status))
                return 1
            }
        }
    }

    /// POST a JSON body and return the status + parsed object, or nil on a
    /// transport error. Uses the loose Json helpers to match the client's style.
    private static func post(_ urlString: String, body: [String: Any]) async -> (status: Int, json: [String: Any]?)? {
        guard let url = URL(string: urlString) else { return nil }
        var req = URLRequest(url: url)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = Data(Json.string(body).utf8)
        guard let (data, resp) = try? await URLSession.shared.data(for: req) else { return nil }
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        let text = String(data: data, encoding: .utf8) ?? ""
        return (status, Json.object(text))
    }

    // withToken keeps the token in the URL (Prefs stores one combined string that
    // WebSocketClient splits at dial time). The server already appends ?token=, so
    // this only adds it in the unlikely case it's missing.
    private static func withToken(_ wss: String, _ token: String) -> String {
        if token.isEmpty || wss.contains("token=") { return wss }
        let sep = wss.contains("?") ? "&" : "?"
        let enc = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        return wss + sep + "token=" + enc
    }

    private static func serverError(_ json: [String: Any]?, _ status: Int) -> String {
        let msg = json?.string("error") ?? ""
        return msg.isEmpty ? "server said \(status)" : msg
    }

    private static func printErr(_ s: String) {
        FileHandle.standardError.write(Data((s + "\n").utf8))
    }
}
