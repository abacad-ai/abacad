import Foundation

// A short-lived screenshot cache with single-flight coalescing, the desktop
// analogue of the Android client's shot cache (AbacadAccessibilityService).
//
// Two independent optimizations, both keyed off a monotonic screen "generation":
//
//   • Cache — a capture is reused for up to CACHE_WINDOW_NS after it was taken,
//     so the dashboard's periodic poll and an agent's screenshot that land within
//     the same second share one capture + JPEG encode instead of doing two.
//
//   • Single-flight — concurrent screenshot commands (each dispatched on its own
//     detached Task) that miss the cache join one in-flight capture and share its
//     result, rather than each kicking an independent ScreenCaptureKit grab.
//
// Any drive command (tap, click, scroll, input, …) calls invalidate(), bumping
// the generation so the very next screenshot is guaranteed fresh — never a frame
// that predates the action. macOS has no platform screenshot rate limit (unlike
// Android's ~333ms takeScreenshot floor), so there is deliberately no pacing or
// retry logic here; the cache exists purely to avoid redundant work.
actor ScreenshotCache {
    static let shared = ScreenshotCache()

    /// A screenshot requested within this window of the last capture is served
    /// from cache instead of re-capturing.
    private static let CACHE_WINDOW_NS: UInt64 = 1_000_000_000 // 1s

    private struct Entry {
        let shot: ScreenCapture.Shot
        let tree: [String: Any]?
        let stampNs: UInt64
        let gen: Int
    }

    private var gen = 0                      // monotonic screen generation
    private var entry: Entry?                // last successful capture
    private var task: Task<Entry, Error>?    // in-flight single-flight capture
    private var taskGen = -1                 // generation the in-flight capture belongs to
    private var taskHasTree = false          // whether the in-flight capture will carry a UI tree
    private var taskId = 0                    // identity token (Task is a value type, so no ===)

    /// Invalidate the cache after a screen-changing command, so the next
    /// screenshot re-captures. Called for every non-screenshot method.
    func invalidate() {
        gen &+= 1
    }

    /// Serve a screenshot as the wire result dict, reusing a cached or in-flight
    /// capture when possible. `includeTree` requires the served frame to carry a
    /// UI tree; a cached frame without one (or an in-flight capture not taking
    /// one) does not satisfy a tree request.
    func screenshot(includeTree: Bool) async throws -> [String: Any] {
        let now = DispatchTime.now().uptimeNanoseconds
        if let e = entry, e.gen == gen,
           now &- e.stampNs < Self.CACHE_WINDOW_NS,
           !includeTree || e.tree != nil {
            return response(e, includeTree: includeTree)
        }
        let e = try await capture(includeTree: includeTree)
        return response(e, includeTree: includeTree)
    }

    /// Join a compatible in-flight capture, or kick a fresh one. Coalescing is
    /// safe under actor reentrancy: while the creator awaits the Task, later
    /// callers re-enter, see the same `task` for this generation, and await it too.
    private func capture(includeTree: Bool) async throws -> Entry {
        if let t = task, taskGen == gen, !includeTree || taskHasTree {
            return try await t.value
        }
        let capGen = gen
        let wantTree = includeTree
        let t = Task { () throws -> Entry in
            let shot = try await ScreenCapture.capture()
            // Capture the tree alongside the pixels so both describe the same
            // screen state; AccessibilityTree.capture() is synchronous.
            let tree = wantTree ? AccessibilityTree.capture() : nil
            return Entry(shot: shot, tree: tree,
                         stampNs: DispatchTime.now().uptimeNanoseconds, gen: capGen)
        }
        taskId &+= 1
        let myId = taskId
        task = t; taskGen = capGen; taskHasTree = wantTree
        do {
            let e = try await t.value
            // Clear only if no newer capture replaced ours — so a completed task
            // can't be joined as if still in flight, and we don't clobber a
            // successor. (Task is a value type; identity is via this token.)
            if taskId == myId { task = nil }
            // Only cache if no drive command bumped the generation mid-capture;
            // otherwise the frame predates that action and must not be reused.
            if e.gen == gen { entry = e }
            return e
        } catch {
            if taskId == myId { task = nil }
            throw error
        }
    }

    /// Build the wire result from a captured frame, attaching the UI tree only
    /// when asked (and present). Field stays `png_base64` for wire compatibility
    /// even though the bytes are JPEG — see ScreenCapture.
    private func response(_ e: Entry, includeTree: Bool) -> [String: Any] {
        var result: [String: Any] = ["w": e.shot.w, "h": e.shot.h, "png_base64": e.shot.base64]
        if includeTree, let tree = e.tree { result["tree"] = tree }
        return result
    }
}
