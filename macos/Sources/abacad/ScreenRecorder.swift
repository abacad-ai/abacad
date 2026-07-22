import ScreenCaptureKit
import AVFoundation
import CoreMedia
import AppKit
import Foundation

// The macOS file channel of screen_recording: a continuous capture of the main
// display, recorded to a local H.264 .mp4 at native pixel resolution (best
// quality — this is an artifact, not a coordinate-mapped frame like screenshot),
// then uploaded to /blobs on stop. Video-only for now; audio is a follow-up.
//
// Capture uses ScreenCaptureKit's SCStream (the streaming counterpart of the
// SCScreenshotManager used by ScreenCapture) feeding CMSampleBuffers into an
// AVAssetWriter. The transfer is async: stop() finalizes the file fast and hands
// the upload to a detached task, so a big clip never blocks the command window —
// the agent polls status() until the blob id appears. One recording at a time.
//
// ScreenRecorder is an actor so its lifecycle/state is race-free; the per-frame
// append work happens on RecordingSession's own serial queue (below).
actor ScreenRecorder {
    static let shared = ScreenRecorder()

    private enum Phase: String {
        case idle, recording, uploading, ready, failed
    }

    private var phase: Phase = .idle
    private var session: RecordingSession?
    private var blobs: BlobClient?
    private var startedAt: Date?
    private var width = 0
    private var height = 0
    private var fps = 0
    private var durationMs: Int64 = 0
    private var sizeBytes: Int64 = 0
    private var blobID = ""
    private var sha256 = ""
    private var errorText = ""
    private var capURL: URL?
    private var capTask: Task<Void, Never>?

    // MARK: Lifecycle

    /// Begin recording. Requires the /blobs client (the point is to transfer the
    /// file afterward). Rejects a second concurrent recording and audio (not yet
    /// implemented). fps == 0 means "native/max".
    func start(blobs: BlobClient, fps requestedFPS: Int, audio: Bool,
               maxDurationSeconds: Int) async throws -> [String: Any] {
        if phase == .recording {
            throw CmdError.message("a recording is already in progress; stop it first")
        }
        if audio {
            throw CmdError.message("audio capture is not yet implemented — record video-only for now")
        }

        // Resolve the main display and capture at its full reported size (best
        // quality — this artifact isn't coordinate-mapped like screenshot). Round
        // to even dimensions, which H.264 requires.
        let content = try await SCShareableContent.excludingDesktopWindows(false, onScreenWindowsOnly: false)
        guard let display = content.displays.first else {
            throw CmdError.message("no display available")
        }
        let w = display.width & ~1
        let h = display.height & ~1
        let rate = requestedFPS > 0 ? requestedFPS : 60

        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("abacad-rec-\(UUID().uuidString).mp4")

        let filter = SCContentFilter(display: display, excludingWindows: [])
        let config = SCStreamConfiguration()
        config.width = w
        config.height = h
        config.minimumFrameInterval = CMTime(value: 1, timescale: CMTimeScale(rate))
        config.showsCursor = true
        config.pixelFormat = kCVPixelFormatType_32BGRA
        config.queueDepth = 6

        let sess = try RecordingSession(url: url, width: w, height: h)
        try await sess.startCapture(filter: filter, config: config)

        // Commit state.
        session = sess
        self.blobs = blobs
        startedAt = Date()
        width = w; height = h; fps = rate
        durationMs = 0; sizeBytes = 0
        blobID = ""; sha256 = ""; errorText = ""
        capURL = url
        phase = .recording

        // Best-effort safety cap: auto-stop after maxDurationSeconds.
        capTask?.cancel()
        if maxDurationSeconds > 0 {
            capTask = Task { [weak self] in
                try? await Task.sleep(nanoseconds: UInt64(maxDurationSeconds) * 1_000_000_000)
                if Task.isCancelled { return }
                _ = await self?.stop()
            }
        }

        return ["state": Phase.recording.rawValue, "width": w, "height": h, "fps": rate]
    }

    /// Stop the active recording, finalize the file, and kick off the background
    /// upload. Returns immediately with state=uploading; the agent polls status().
    /// A stop with nothing recording is a no-op reporting the current state.
    func stop() async -> [String: Any] {
        guard phase == .recording, let sess = session else {
            return status()
        }
        capTask?.cancel(); capTask = nil

        let size = await sess.finish()
        session = nil
        durationMs = Int64(Date().timeIntervalSince(startedAt ?? Date()) * 1000)
        sizeBytes = size
        phase = .uploading

        // Upload off the command path; flip to ready/failed when it settles.
        let url = capURL
        let client = blobs
        Task { [weak self] in
            guard let self, let url, let client else {
                await self?.markUploadFailed("file transfer is not configured")
                return
            }
            do {
                let (id, _, sha) = try await client.upload(srcPath: url.path)
                try? FileManager.default.removeItem(at: url) // auto-retention: keep the store copy
                await self.markUploaded(id: id, sha: sha)
            } catch {
                await self.markUploadFailed("\(error)")
            }
        }

        var out = status()
        out["state"] = Phase.uploading.rawValue
        return out
    }

    /// Report the current recording/transfer state.
    func status() -> [String: Any] {
        var out: [String: Any] = ["state": phase.rawValue]
        if width > 0 { out["width"] = width }
        if height > 0 { out["height"] = height }
        if fps > 0 { out["fps"] = fps }
        switch phase {
        case .recording:
            out["elapsed_ms"] = Int64(Date().timeIntervalSince(startedAt ?? Date()) * 1000)
            out["size_bytes"] = capURL.map { fileSize($0) } ?? 0
        case .uploading, .ready, .failed:
            out["duration_ms"] = durationMs
            out["size_bytes"] = sizeBytes
            out["codec"] = "h264"
            out["transfer_state"] = phase == .ready ? "ready" : (phase == .failed ? "failed" : "uploading")
            if !blobID.isEmpty { out["blob_id"] = blobID }
            if !sha256.isEmpty { out["sha256"] = sha256 }
            if !errorText.isEmpty { out["error"] = errorText }
        case .idle:
            break
        }
        return out
    }

    // MARK: Upload callbacks (actor-isolated state mutation)

    private func markUploaded(id: String, sha: String) {
        blobID = id; sha256 = sha; phase = .ready
    }

    private func markUploadFailed(_ msg: String) {
        errorText = msg; phase = .failed
    }

    private func fileSize(_ url: URL) -> Int64 {
        ((try? FileManager.default.attributesOfItem(atPath: url.path))?[.size] as? NSNumber)?.int64Value ?? 0
    }
}

// Owns one AVAssetWriter + SCStream for a single recording. All AVAssetWriter
// mutation happens on `queue` (also the SCStream sample-handler queue), so it is
// single-threaded without extra locking. NSObject/@objc so it can be an
// SCStreamOutput + SCStreamDelegate. @unchecked Sendable: all mutable state
// (writer, input, started) is touched only on `queue`.
private final class RecordingSession: NSObject, SCStreamOutput, SCStreamDelegate, @unchecked Sendable {
    private let url: URL
    private let queue = DispatchQueue(label: "ai.abacad.screenrec.samples")
    private let writer: AVAssetWriter
    private let input: AVAssetWriterInput
    private var stream: SCStream?
    private var started = false

    init(url: URL, width: Int, height: Int) throws {
        self.url = url
        writer = try AVAssetWriter(outputURL: url, fileType: .mp4)
        input = AVAssetWriterInput(mediaType: .video, outputSettings: [
            AVVideoCodecKey: AVVideoCodecType.h264,
            AVVideoWidthKey: width,
            AVVideoHeightKey: height,
        ])
        input.expectsMediaDataInRealTime = true
        super.init()
        writer.add(input)
    }

    func startCapture(filter: SCContentFilter, config: SCStreamConfiguration) async throws {
        let s = SCStream(filter: filter, configuration: config, delegate: self)
        try s.addStreamOutput(self, type: .screen, sampleHandlerQueue: queue)
        try await s.startCapture()
        stream = s
    }

    /// Stop the stream and finalize the file. Returns the written byte size.
    func finish() async -> Int64 {
        try? await stream?.stopCapture()
        stream = nil
        await withCheckedContinuation { (cont: CheckedContinuation<Void, Never>) in
            queue.async {
                if self.writer.status == .writing {
                    self.input.markAsFinished()
                    self.writer.finishWriting { cont.resume() }
                } else {
                    cont.resume()
                }
            }
        }
        return ((try? FileManager.default.attributesOfItem(atPath: url.path))?[.size] as? NSNumber)?.int64Value ?? 0
    }

    // SCStreamOutput: append each complete frame. Runs on `queue`.
    func stream(_ stream: SCStream, didOutputSampleBuffer sampleBuffer: CMSampleBuffer,
                of type: SCStreamOutputType) {
        guard type == .screen, sampleBuffer.isValid else { return }
        // Skip idle/blank frames — only .complete frames carry pixels.
        guard let attach = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, createIfNecessary: false)
                as? [[SCStreamFrameInfo: Any]],
              let statusRaw = attach.first?[.status] as? Int,
              let status = SCFrameStatus(rawValue: statusRaw), status == .complete else { return }

        if !started {
            guard writer.startWriting() else { return }
            writer.startSession(atSourceTime: CMSampleBufferGetPresentationTimeStamp(sampleBuffer))
            started = true
        }
        if input.isReadyForMoreMediaData {
            input.append(sampleBuffer)
        }
    }
}
