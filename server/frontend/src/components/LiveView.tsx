import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { api } from "../lib/api";

type State = "idle" | "connecting" | "connected" | "error";

// LiveView is the "Screen Recording" hero on the device detail page: a real-time
// view of the device screen (and, with a VNC client, control), served over the
// decoupled VNC path. POST vnc/start mints a viewer ticket and tells the device to
// start its VNC server + reverse-connect; stock noVNC (loaded lazily) then connects
// to /vnc/watch?ticket=… (the browser's session cookie authorizes the watch).
//
// It's mounted only while the Screen Recording tab is active, so it auto-connects
// on mount and tears the session down on unmount (tab switch / leaving the page).
export function LiveView({ deviceId, online }: { deviceId: string; online: boolean }) {
  const [state, setState] = useState<State>("idle");
  const [err, setErr] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rfbRef = useRef<any>(null);

  const disconnect = useCallback(() => {
    if (rfbRef.current) {
      try {
        rfbRef.current.disconnect();
      } catch {
        /* already gone */
      }
      rfbRef.current = null;
    }
  }, []);

  const start = useCallback(async () => {
    setErr("");
    setState("connecting");
    try {
      const { watch_path } = await api.vncStart(deviceId);
      const proto = location.protocol === "https:" ? "wss:" : "ws:";
      const url = `${proto}//${location.host}${watch_path}`;
      const { default: RFB } = await import("@novnc/novnc");
      const rfb = new RFB(containerRef.current!, url);
      rfb.scaleViewport = true;
      rfb.background = "transparent";
      rfb.addEventListener("connect", () => setState("connected"));
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      rfb.addEventListener("disconnect", (e: any) => {
        rfbRef.current = null;
        if (e?.detail?.clean) {
          setState("idle");
        } else {
          setErr("live view disconnected");
          setState("error");
        }
      });
      rfb.addEventListener("securityfailure", () => {
        setErr("live view authentication failed");
        setState("error");
      });
      rfbRef.current = rfb;
    } catch (e) {
      setErr(e instanceof Error ? e.message : "could not start live view");
      setState("error");
    }
  }, [deviceId]);

  // Auto-connect on mount (tab selected); tear the session down on unmount.
  useEffect(() => {
    if (online) start();
    return () => {
      disconnect();
      api.vncStop(deviceId).catch(() => {});
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return (
    <div className="relative overflow-hidden rounded-[12px] border border-border bg-black/80">
      {/* noVNC mounts its canvas here; shown only once connected so the placeholder
          states below don't overlap it. */}
      <div
        ref={containerRef}
        className="min-h-[320px] w-full"
        style={{ display: state === "connected" ? "block" : "none" }}
      />
      {state !== "connected" && (
        <div className="flex min-h-[320px] items-center justify-center p-6 text-center text-sm text-ink-muted">
          {!online ? (
            <span>Device is offline.</span>
          ) : state === "error" ? (
            <div className="space-y-3">
              <p className="text-red-400">{err}</p>
              <button
                type="button"
                onClick={start}
                className="rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-ink hover:bg-surface-2"
              >
                Retry
              </button>
            </div>
          ) : (
            <span className="inline-flex items-center gap-2">
              <Loader2 className="h-4 w-4 animate-spin" />
              {state === "connecting" ? "Connecting…" : "Starting live view…"}
            </span>
          )}
        </div>
      )}
    </div>
  );
}
