import { useEffect, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Plus, RefreshCw, Trash2, Pencil, Smartphone, ImageOff } from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

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
// fetch is a round-trip to the phone, so it auto-refreshes ~10s AFTER the previous
// capture settles (never firing a new request while one is in flight) and lets
// the user refresh on demand. Offline devices skip it.
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
        timer = setTimeout(loadNext, 10000);
      };
      img.onerror = () => {
        if (!alive) return;
        setFailed(true); // ignored once we have a frame; keeps the old one up
        timer = setTimeout(loadNext, 10000);
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

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);

  const reload = async () => {
    try {
      setDevices(await api.devices());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
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
                    <div className="truncate font-mono text-xs text-slate-500">{d.id}</div>
                  </div>
                </div>
                <div className="flex shrink-0 items-center">
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
    </div>
  );
}
