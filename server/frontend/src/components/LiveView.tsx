import { useCallback, useEffect, useRef, useState } from "react";
import { Loader2, Radio, X } from "lucide-react";
import { api } from "../lib/api";

type State = "idle" | "connecting" | "connected" | "error";

// LiveView is the noVNC panel on the device detail page: a real-time view of the
// device screen (and, with a VNC client, control), served over the decoupled VNC
// path — POST vnc/start mints a viewer ticket and tells the device to start its
// VNC server + reverse-connect, then stock noVNC connects to /vnc/watch?ticket=…
// (the browser's session cookie authorizes the watch). noVNC is loaded lazily so
// its bundle only ships when someone opens a live session.
export function LiveView({ deviceId, online }: { deviceId: string; online: boolean }) {
  const [state, setState] = useState<State>("idle");
  const [err, setErr] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rfbRef = useRef<any>(null);

  const teardown = useCallback(
    (stopServer: boolean) => {
      if (rfbRef.current) {
        try {
          rfbRef.current.disconnect();
        } catch {
          /* already gone */
        }
        rfbRef.current = null;
      }
      if (stopServer) api.vncStop(deviceId).catch(() => {});
    },
    [deviceId],
  );

  // Always tear the session down when leaving the page.
  useEffect(() => () => teardown(true), [teardown]);

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

  const stop = useCallback(() => {
    teardown(true);
    setState("idle");
  }, [teardown]);

  const active = state === "connecting" || state === "connected";

  return (
    <section className="mt-8">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-semibold text-ink">
          <Radio className="h-4 w-4 text-ink-muted" />
          Live view
        </h2>
        {active ? (
          <button
            onClick={stop}
            className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-ink-muted hover:text-ink"
          >
            <X className="h-3.5 w-3.5" />
            Stop
          </button>
        ) : (
          <button
            onClick={start}
            disabled={!online}
            className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-surface px-3 py-1.5 text-xs font-medium text-ink hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50"
          >
            <Radio className="h-3.5 w-3.5" />
            {online ? "Start live view" : "Device offline"}
          </button>
        )}
      </div>

      {/* The noVNC canvas mounts into this container. Kept mounted whenever a
          session is active so the ref is stable across state changes. */}
      <div className="relative overflow-hidden rounded-xl border border-border bg-black/80">
        <div
          ref={containerRef}
          className="min-h-[240px] w-full"
          style={{ display: active ? "block" : "none" }}
        />
        {state !== "connected" && (
          <div className="flex min-h-[240px] items-center justify-center p-6 text-center text-sm text-ink-muted">
            {state === "connecting" && (
              <span className="inline-flex items-center gap-2">
                <Loader2 className="h-4 w-4 animate-spin" />
                Connecting…
              </span>
            )}
            {state === "error" && <span className="text-red-400">{err}</span>}
            {state === "idle" && (
              <span>
                Watch the device in real time. Live view opens a VNC session over an
                encrypted, ticketed connection.
              </span>
            )}
          </div>
        )}
      </div>
    </section>
  );
}
