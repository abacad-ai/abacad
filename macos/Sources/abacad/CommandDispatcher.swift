import Foundation
import CoreGraphics
import ApplicationServices

// Routes a parsed {id, method, params} command to a handler and produces the
// {id, ok, result|error} reply. Correlation is purely by id; replies may be sent
// out of order (screenshots are async). Malformed frames are dropped upstream
// with no reply, matching the Android client.
//
// The Mac answers a superset: the mobile verbs (mapped to desktop equivalents so
// today's tools/agent work unchanged) plus the desktop-native verbs. Anything
// unrecognized returns "unknown method: X" — which is how the server keeps one
// global tool list without per-platform filtering.
struct CommandDispatcher {
    /// Backs push_file / pull_file over the /blobs data plane. Set by Agent from
    /// the server URL; nil disables file transfer (the verbs then say so).
    var blobClient: BlobClient?

    /// Execute a method and return the `result` object, or throw CmdError.
    func execute(method: String, params: [String: Any]) async throws -> [String: Any] {
        // Any non-screenshot command may change the screen, so invalidate the
        // shot cache before running it — the next screenshot must never serve a
        // frame captured before this action. (Matches the Android client.)
        if method != "screenshot" {
            await ScreenshotCache.shared.invalidate()
        }
        switch method {
        case "screenshot":
            return try await ScreenshotCache.shared.screenshot(includeTree: params.bool("include_ui_tree", true))

        // Mobile verbs, mapped onto desktop input for cross-platform compatibility.
        case "tap":
            requireCoords(params, "x", "y")
            InputInjection.click(x: params.int("x"), y: params.int("y"))
            return ["dispatched": true]
        case "long_press":
            requireCoords(params, "x", "y")
            InputInjection.longPress(x: params.int("x"), y: params.int("y"), holdMs: params.int("duration_ms", 600))
            return ["dispatched": true]
        case "swipe":
            InputInjection.drag(x1: params.int("x1"), y1: params.int("y1"),
                                x2: params.int("x2"), y2: params.int("y2"),
                                durationMs: params.int("duration_ms", 300))
            return ["dispatched": true]
        case "input_text":
            return ["set": setFocusedText(params.string("text"))]

        // Desktop-native verbs.
        case "click":
            requireCoords(params, "x", "y")
            InputInjection.click(x: params.int("x"), y: params.int("y"),
                                 button: .left, count: params.int("count", 1),
                                 modifiers: params.strings("modifiers"))
            return ["dispatched": true]
        case "right_click":
            requireCoords(params, "x", "y")
            InputInjection.rightClick(x: params.int("x"), y: params.int("y"))
            return ["dispatched": true]
        case "drag":
            InputInjection.drag(x1: params.int("x1"), y1: params.int("y1"),
                                x2: params.int("x2"), y2: params.int("y2"),
                                durationMs: params.int("duration_ms", 300),
                                modifiers: params.strings("modifiers"))
            return ["dispatched": true]
        case "scroll":
            requireCoords(params, "x", "y")
            InputInjection.scroll(x: params.int("x"), y: params.int("y"),
                                  dx: params.int("dx"), dy: params.int("dy"))
            return ["dispatched": true]
        case "press_keys":
            let keys = params.strings("keys")
            guard !keys.isEmpty else { throw CmdError.message("press_keys requires a non-empty keys array") }
            let pressed = InputInjection.pressChord(keys)
            if !pressed { throw CmdError.message("press_keys: no recognized key in \(keys)") }
            return ["pressed": true]
        case "composite":
            let steps = params.objects("steps")
            guard !steps.isEmpty else { throw CmdError.message("composite requires a non-empty steps array") }
            return try await Composite.run(steps)

        // File transfer over the /blobs data plane. Filesystem I/O, no display or
        // input needed — works the same whether or not anyone is at the screen.
        case "push_file":
            guard let blobs = blobClient else { throw CmdError.message("file transfer is not configured on this device") }
            let blobID = params.string("blob_id"), dest = params.string("dest_path")
            guard !blobID.isEmpty, !dest.isEmpty else { throw CmdError.message("push_file requires blob_id and dest_path") }
            let (size, sha) = try await blobs.download(blobID: blobID, destPath: dest, mode: params.int("mode", 0o644))
            return ["written": true, "size": size, "sha256": sha]
        case "pull_file":
            guard let blobs = blobClient else { throw CmdError.message("file transfer is not configured on this device") }
            let src = params.string("src_path")
            guard !src.isEmpty else { throw CmdError.message("pull_file requires src_path") }
            let (id, size, sha) = try await blobs.upload(srcPath: src)
            return ["blob_id": id, "size": size, "sha256": sha]

        // Screen recording (file channel). Records the display to a high-quality
        // .mp4 on disk, then uploads it via /blobs on stop; the agent fetches the
        // finished clip from GET /blobs/{id}. Needs the data plane to transfer.
        case "screen_recording":
            guard let blobs = blobClient else { throw CmdError.message("screen recording needs the /blobs data plane, which is not configured on this device") }
            let action = params.string("action")
            switch action {
            case "start":
                let file = (params["file"] as? [String: Any]) ?? [:]
                return try await ScreenRecorder.shared.start(
                    blobs: blobs,
                    fps: file.int("fps", 0),
                    maxDurationSeconds: file.int("max_duration_seconds", 0))
            case "stop":
                return await ScreenRecorder.shared.stop()
            case "status":
                return await ScreenRecorder.shared.status()
            default:
                throw CmdError.message(#"screen_recording action must be "start", "stop", or "status""#)
            }

        // Mobile navigation keys have no desktop analogue.
        case "back", "home", "recents":
            throw CmdError.message("\(method) has no desktop analogue — use click / press_keys")

        default:
            throw CmdError.message("unknown method: \(method)")
        }
    }

    // MARK: Handlers

    /// Replace the focused field's contents via AX (matches Android's input_text
    /// "set text" semantics). Falls back to typing if the element rejects the set.
    private func setFocusedText(_ text: String) -> Bool {
        let system = AXUIElementCreateSystemWide()
        var focused: CFTypeRef?
        guard AXUIElementCopyAttributeValue(system, kAXFocusedUIElementAttribute as CFString, &focused) == .success,
              let element = focused else {
            InputInjection.typeText(text)
            return true
        }
        let el = element as! AXUIElement
        let err = AXUIElementSetAttributeValue(el, kAXValueAttribute as CFString, text as CFString)
        if err != .success {
            InputInjection.typeText(text)
        }
        return true
    }

    private func requireCoords(_ p: [String: Any], _ keys: String...) {
        // Loose: missing keys default to 0. Kept as a hook for future validation;
        // desktop coords can legitimately be 0, so we don't reject negatives the
        // way the phone does.
        _ = keys
    }
}
