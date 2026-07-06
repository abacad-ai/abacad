import { useEffect, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  Plus,
  RefreshCw,
  Trash2,
  Pencil,
  Smartphone,
  ImageOff,
  Activity,
} from "lucide-react";
import { api, type DeviceView, type DeviceEvent } from "@/lib/api";
import { relativeTime, clockTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

// How often the device list re-polls, so online/offline and last-seen stay live
// instead of frozen at mount.
const DEVICES_POLL_MS = 5000;

// The gap between one screenshot finishing and the next one starting. The next
// fetch is scheduled only inside onload/onerror, so this is a true idle gap after
// each capture settles — not a fixed polling period — and requests never stack.
// The client owns this cadence (the device is stateless).
const SCREENSHOT_GAP_MS = 2000;

interface Reveal {
  title: string;
  wssUrl: string;
  token: string;
}

// The device connects through the same origin as this dashboard (Vite's /device
// proxy in dev, Go in prod), so derive the URL from the browser's location
// rather than the backend's view of Host — which is "localhost:1213" behind the
// dev proxy and wouldn't be reachable from another device on the LAN.
function deviceWsUrl(token: string): string {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}/device?token=${token}`;
}

// DeviceScreenshot is the card cover: a live PNG pulled from the device. Each
// fetch is a round-trip to the phone; when one settles we wait SCREENSHOT_GAP_MS
// and fetch again (never firing a new request while one is in flight) and let
// the user refresh on demand. Offline devices skip it.
//
// The device itself is honest and stateless — it takes one shot per request and
// reports a failure (e.g. the ~333ms accessibility rate limit) rather than retrying.
// Pacing lives here, on the client, so the cadence is a UI concern, not device state.
//
// Frames are preloaded off-screen and only swapped into the visible <img> once
// they actually decode, so a failed capture leaves the last good frame on screen
// instead of flashing a broken image.
function DeviceScreenshot({ device }: { device: DeviceView }) {
  // src is the last frame that successfully loaded (null until the first one).
  const [src, setSrc] = useState<string | null>(null);
  // failed only drives the first-load error state; once we have a frame, later
  // failures are silent (we keep showing src).
  const [failed, setFailed] = useState(false);
  // Bumping this restarts the capture loop for a manual refresh.
  const [manualNonce, setManualNonce] = useState(0);

  useEffect(() => {
    if (!device.online) {
      setSrc(null);
      setFailed(false);
      return;
    }
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    let seq = 0;

    const loadNext = () => {
      // Unique query string → the no-store endpoint captures a fresh frame.
      const url = `${api.deviceScreenshotUrl(device.id)}?t=${Date.now()}_${seq++}`;
      const img = new Image();
      img.onload = () => {
        if (!alive) return;
        setSrc(url); // already decoded → visible swap is instant, no flicker
        setFailed(false);
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.onerror = () => {
        if (!alive) return;
        setFailed(true); // ignored once we have a frame; keeps the old one up
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.src = url;
    };
    loadNext();

    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [device.online, device.id, manualNonce]);

  const refresh = () => setManualNonce((n) => n + 1);

  return (
    <div className="group relative aspect-[9/16] overflow-hidden rounded-lg bg-slate-800/60">
      {device.online ? (
        <>
          {src && (
            <img src={src} alt={`${device.name} screen`} className="h-full w-full object-cover" />
          )}
          {!src && !failed && (
            <div className="absolute inset-0 flex items-center justify-center bg-slate-800/60">
              <RefreshCw size={20} className="animate-spin text-slate-500" />
            </div>
          )}
          {!src && failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-1 text-slate-500">
              <ImageOff size={22} />
              <span className="text-xs">Couldn’t capture</span>
            </div>
          )}
          <button
            onClick={refresh}
            title="Refresh screenshot"
            className="absolute bottom-2 right-2 rounded-md bg-black/50 p-1.5 text-white opacity-0 transition-opacity hover:bg-black/70 group-hover:opacity-100"
          >
            <RefreshCw size={14} />
          </button>
        </>
      ) : (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 text-slate-600">
          <Smartphone size={26} />
          <span className="text-xs text-slate-500">Offline</span>
        </div>
      )}
    </div>
  );
}

// How often the activity feed re-polls while its modal is open.
const ACTIVITY_POLL_MS = 3000;

// outcomeStyle colors a command row by how it ended: green ok, amber for a
// dropped/canceled connection, red for a timeout or error.
function outcomeStyle(outcome?: string): { dot: string; text: string } {
  switch (outcome) {
    case "ok":
      return { dot: "bg-emerald-400", text: "text-emerald-300" };
    case "timeout":
    case "error":
      return { dot: "bg-red-400", text: "text-red-300" };
    case "device_gone":
    case "canceled":
      return { dot: "bg-amber-400", text: "text-amber-300" };
    default:
      return { dot: "bg-slate-500", text: "text-slate-300" };
  }
}

// eventLabel renders one activity row's human text.
function eventLabel(e: DeviceEvent): string {
  if (e.kind === "connected") return "Connected";
  if (e.kind === "disconnected") return `Disconnected${e.detail ? ` — ${e.detail}` : ""}`;
  // command
  const src = e.source ? `${e.source} · ` : "";
  const dur = e.duration_ms != null ? ` · ${e.duration_ms}ms` : "";
  const outcome = e.outcome ?? "?";
  const detail = e.outcome === "error" && e.detail ? ` — ${e.detail}` : "";
  return `${src}${e.method}${dur} · ${outcome}${detail}`;
}

// eventDot picks the status color for a row, keyed on kind then command outcome.
function eventDot(e: DeviceEvent): string {
  if (e.kind === "connected") return "bg-emerald-400";
  if (e.kind === "disconnected") return "bg-amber-400";
  return outcomeStyle(e.outcome).dot;
}

// DeviceActivity is the per-device activity feed: recent connects, disconnects
// (with reason), and every command with its source, duration, and outcome. It
// polls while open so a live session updates in place — the visual answer to
// "why did that time out?" / "why did it drop?".
function DeviceActivity({ device, onClose }: { device: DeviceView; onClose: () => void }) {
  const [events, setEvents] = useState<DeviceEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;
    const load = async () => {
      try {
        const r = await api.deviceEvents(device.id);
        if (!alive) return;
        setEvents(r.events);
        setError(null);
      } catch (e) {
        if (alive) setError((e as Error).message);
      } finally {
        if (alive) timer = setTimeout(load, ACTIVITY_POLL_MS);
      }
    };
    void load();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [device.id]);

  return (
    <Modal open onClose={onClose} title={`Activity — ${device.name}`}>
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-2 text-xs text-slate-400">
          <span
            className={`h-2 w-2 rounded-full ${device.online ? "bg-emerald-400" : "bg-slate-600"}`}
          />
          {device.online ? "Online" : "Offline"}
          {device.last_seen && !device.online && (
            <span>· last seen {relativeTime(device.last_seen)}</span>
          )}
        </div>

        {error && <p className="text-sm text-red-400">{error}</p>}

        {events === null ? (
          <p className="text-sm text-slate-500">Loading…</p>
        ) : events.length === 0 ? (
          <p className="text-sm text-slate-500">
            No activity yet. Events appear here as the device connects and the agent drives it.
          </p>
        ) : (
          <ul className="max-h-80 divide-y divide-slate-800/70 overflow-y-auto rounded-lg border border-slate-800 text-sm">
            {events.map((e, i) => (
              <li key={`${e.ts}-${i}`} className="flex items-start gap-2.5 px-3 py-2">
                <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${eventDot(e)}`} />
                <div className="min-w-0 flex-1">
                  <div
                    className={`truncate ${
                      e.kind === "command" ? outcomeStyle(e.outcome).text : "text-slate-200"
                    }`}
                  >
                    {eventLabel(e)}
                  </div>
                </div>
                <span className="shrink-0 font-mono text-xs text-slate-500">
                  {clockTime(e.ts)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);
  // Which device's activity feed is open (by id, so it tracks list updates).
  const [activityId, setActivityId] = useState<string | null>(null);
  const loadedOnce = useRef(false);

  const reload = async () => {
    try {
      setDevices(await api.devices());
      setError(null);
    } catch (e) {
      // Don't blank the list on a transient poll failure once we have data.
      if (!loadedOnce.current) setError((e as Error).message);
    } finally {
      loadedOnce.current = true;
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
    // Re-poll so online/offline and last-seen stay live without a manual refresh.
    const t = setInterval(() => void reload(), DEVICES_POLL_MS);
    return () => clearInterval(t);
  }, []);

  const addDevice = async () => {
    const name = window.prompt("Name this device", "My phone");
    if (name === null) return;
    const d = await api.createDevice(name || "New device");
    setReveal({ title: `Connect “${d.name}”`, wssUrl: deviceWsUrl(d.device_token), token: d.device_token });
    void reload();
  };

  const rename = async (d: DeviceView) => {
    const name = window.prompt("Rename device", d.name);
    if (!name || name === d.name) return;
    await api.renameDevice(d.id, name);
    void reload();
  };

  const remove = async (d: DeviceView) => {
    if (!window.confirm(`Remove “${d.name}”? Its token stops working immediately.`)) return;
    await api.deleteDevice(d.id);
    void reload();
  };

  const rotate = async (d: DeviceView) => {
    if (!window.confirm(`Rotate the token for “${d.name}”? The current one stops working.`)) return;
    const r = await api.rotateDeviceToken(d.id);
    setReveal({ title: `New token for “${d.name}”`, wssUrl: deviceWsUrl(r.device_token), token: r.device_token });
  };

  return (
    <div>
      <div className="mb-5 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold">Devices</h1>
          <p className="text-sm text-slate-400">Phones and machines your agents can drive.</p>
        </div>
        <Button onClick={addDevice}>
          <Plus size={16} /> Add device
        </Button>
      </div>

      {loading ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : error ? (
        <p className="text-sm text-red-400">{error}</p>
      ) : devices.length === 0 ? (
        <Card className="p-8 text-center">
          <p className="text-sm text-slate-400">No devices yet.</p>
          <p className="mt-1 text-sm text-slate-500">Add one, then paste its URL into the Abacad app.</p>
        </Card>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-4">
          {devices.map((d) => (
            <Card key={d.id} className="flex flex-col gap-3 p-3">
              <DeviceScreenshot device={d} />
              <div className="flex items-start justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <span
                    className={`h-2.5 w-2.5 shrink-0 rounded-full ${d.online ? "bg-emerald-400" : "bg-slate-600"}`}
                    title={d.online ? "online" : "offline"}
                  />
                  <div className="min-w-0">
                    <div className="truncate font-medium text-slate-100">{d.name}</div>
                    <div className="truncate text-xs text-slate-500">
                      {d.online
                        ? "online"
                        : d.last_seen
                          ? `offline · last seen ${relativeTime(d.last_seen)}`
                          : "offline · never connected"}
                    </div>
                  </div>
                </div>
                <div className="flex shrink-0 items-center">
                  <Button variant="ghost" size="icon" onClick={() => setActivityId(d.id)} title="Activity">
                    <Activity size={15} />
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => rename(d)} title="Rename">
                    <Pencil size={15} />
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => rotate(d)} title="Rotate token">
                    <RefreshCw size={15} />
                  </Button>
                  <Button variant="ghost" size="icon" onClick={() => remove(d)} title="Remove">
                    <Trash2 size={15} />
                  </Button>
                </div>
              </div>
            </Card>
          ))}
        </div>
      )}

      <Modal open={reveal !== null} onClose={() => setReveal(null)} title={reveal?.title ?? ""}>
        {reveal && (
          <div className="flex flex-col gap-4">
            <p className="text-sm text-slate-400">
              Paste this URL into the Abacad app on the device, or scan the QR. Shown once — copy it now.
            </p>
            <div className="flex justify-center rounded-xl bg-white p-4">
              <QRCodeSVG value={reveal.wssUrl} size={168} />
            </div>
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Connection URL</div>
              <CopyField value={reveal.wssUrl} />
            </div>
            <div>
              <div className="mb-1 text-xs font-medium uppercase tracking-wide text-slate-500">Device token</div>
              <CopyField value={reveal.token} />
            </div>
          </div>
        )}
      </Modal>

      {activityId &&
        (() => {
          const d = devices.find((x) => x.id === activityId);
          return d ? <DeviceActivity device={d} onClose={() => setActivityId(null)} /> : null;
        })()}
    </div>
  );
}
