import Foundation

// Device-first self-enrollment: the Mac announces itself to a relay, receives its
// permanent id and token, and shows that id plus a rotating claim code in the
// menu-bar panel until a human binds it to an account at <relay>/claim.
//
// The peer of linux/internal/enroll (the reference implementation) and of
// ConnectFlow in Connect.swift, which remains the enrollment path for anyone who
// can't use this one. The wire contract is frozen against the Linux client:
//
//   POST /api/devices/register        -> {device_id, device_token, claim_code, …}
//   POST /api/devices/self/heartbeat  -> {claimed, claim_code, …} | {claimed, claimed_by}
//
// The token survives the claim unchanged, so the unclaimed -> claimed transition
// needs no re-key: the same credential that polls the heartbeat later dials
// /device.
public enum Enrollment {
    public static let defaultRelay = "https://abacad.ai"

    /// Terminal outcomes the caller has to act on differently from a retry.
    public enum EnrollError: Error {
        /// The relay doesn't recognize our token — the registration was reaped or
        /// the device was deleted. Wipe and register again.
        case unknownToken
        /// The relay predates self-enrollment. Fall back to `abacad connect`
        /// rather than failing: a self-hoster who upgrades clients before their
        /// server should get a message, not a brick.
        case notSupported
        case transport(String)
    }

    public struct Registration {
        public let deviceID: String
        public let deviceToken: String
        public let claimCode: String
        public let claimExpiresIn: Int
        public let heartbeatIn: Int
    }

    public struct Status {
        public let claimed: Bool
        public let deviceID: String
        public let claimCode: String
        public let claimExpiresIn: Int
        public let heartbeatIn: Int
        public let name: String
        /// The account that claimed this device. Shown in the panel so a claim by
        /// someone who read the screen isn't silent to the person sitting here.
        public let claimedBy: String
    }

    // MARK: - Calls

    public static func register(relay: String, platform: String, name: String, version: String) async throws -> Registration {
        let (status, json) = try await post(
            url: normalize(relay) + "/api/devices/register",
            body: ["platform": platform, "name": name, "version": version])
        if status == 404 || status == 405 { throw EnrollError.notSupported }
        guard status == 201 || status == 200, let j = json else {
            throw EnrollError.transport(serverError(json, status))
        }
        let id = j.string("device_id"), token = j.string("device_token")
        guard !id.isEmpty, !token.isEmpty else {
            throw EnrollError.transport("relay returned an incomplete registration")
        }
        return Registration(deviceID: id, deviceToken: token, claimCode: j.string("claim_code"),
                            claimExpiresIn: j.int("claim_expires_in"), heartbeatIn: j.int("heartbeat_in"))
    }

    public static func heartbeat(relay: String, token: String) async throws -> Status {
        // Header only: these endpoints are new, so there's no legacy ?token= shape
        // to support and no reason to put the secret in a URL or a proxy log.
        let (status, json) = try await post(
            url: normalize(relay) + "/api/devices/self/heartbeat", body: nil,
            bearer: token)
        if status == 404 || status == 410 || status == 401 { throw EnrollError.unknownToken }
        guard status == 200, let j = json else {
            throw EnrollError.transport(serverError(json, status))
        }
        return Status(claimed: j.bool("claimed"), deviceID: j.string("device_id"),
                      claimCode: j.string("claim_code"), claimExpiresIn: j.int("claim_expires_in"),
                      heartbeatIn: j.int("heartbeat_in"), name: j.string("name"),
                      claimedBy: j.string("claimed_by"))
    }

    // MARK: - Helpers

    /// The WebSocket URL to dial once claimed. http(s) maps to ws(s) so a
    /// loopback dev relay stays on plain ws while a real deployment gets wss —
    /// matching WebSocketClient's own refusal to speak ws:// off-loopback.
    public static func deviceURL(relay: String) -> String {
        var base = normalize(relay)
        if base.hasPrefix("https://") { base = "wss://" + base.dropFirst("https://".count) }
        else if base.hasPrefix("http://") { base = "ws://" + base.dropFirst("http://".count) }
        return base + "/device"
    }

    /// Clamp a server-advertised second count so a missing or hostile hint can't
    /// spin the client.
    public static func interval(_ seconds: Int) -> UInt64 {
        if seconds < 5 { return 20 }
        if seconds > 300 { return 300 }
        return UInt64(seconds)
    }

    public static func normalize(_ relay: String) -> String {
        var s = relay.trimmingCharacters(in: .whitespaces)
        while s.hasSuffix("/") { s.removeLast() }
        return s.isEmpty ? defaultRelay : s
    }

    /// A human-recognizable default name, so the claim page shows something
    /// better than "New device". The claimer can override it.
    public static func defaultDeviceName() -> String {
        let n = Host.current().localizedName ?? ""
        return n.isEmpty ? "Mac" : n
    }

    /// The version reported at registration. Same source WebSocketClient uses for
    /// the ?version= dial parameter — stamped from the root VERSION file.
    ///
    /// Info.plist first, so the .app reports exactly what `make macos` stamped
    /// into the bundle it is running from. The CLI has no bundle, so it falls
    /// back to the compiled-in constant rather than calling itself "dev".
    public static func appVersion() -> String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? AbacadVersion.current
    }

    private static func post(url: String, body: [String: Any]?, bearer: String? = nil)
        async throws -> (Int, [String: Any]?)
    {
        guard let u = URL(string: url) else { throw EnrollError.transport("bad relay URL") }
        var req = URLRequest(url: u)
        req.httpMethod = "POST"
        req.timeoutInterval = 30
        if let body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = Data(Json.string(body).utf8)
        }
        if let bearer { req.setValue("Bearer " + bearer, forHTTPHeaderField: "Authorization") }
        guard let (data, resp) = try? await URLSession.shared.data(for: req) else {
            throw EnrollError.transport("could not reach \(url)")
        }
        let status = (resp as? HTTPURLResponse)?.statusCode ?? 0
        return (status, Json.object(String(data: data, encoding: .utf8) ?? ""))
    }

    private static func serverError(_ json: [String: Any]?, _ status: Int) -> String {
        let msg = json?.string("error") ?? ""
        return msg.isEmpty ? "server said \(status)" : msg
    }
}
