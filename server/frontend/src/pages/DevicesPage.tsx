import { useEffect, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  Activity,
  CheckCircle2,
  Clock3,
  ImageOff,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Trash2,
  Wifi,
  WifiOff,
} from "lucide-react";
import { api, type DeviceEvent, type DeviceView } from "@/lib/api";
import { clockTime, relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

const DEVICES_POLL_MS = 5000;
const SCREENSHOT_GAP_MS = 2000;
const ACTIVITY_POLL_MS = 3000;

interface Reveal {
  title: string;
  wssUrl: string;
  token: string;
}

function deviceWsUrl(token: string): string {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}/device?token=${token}`;
}

function DeviceScreenshot({ device }: { device: DeviceView }) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
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
      const url = `${api.deviceScreenshotUrl(device.id)}?t=${Date.now()}_${seq++}`;
      const img = new Image();
      img.onload = () => {
        if (!alive) return;
        setSrc(url);
        setFailed(false);
        timer = setTimeout(loadNext, SCREENSHOT_GAP_MS);
      };
      img.onerror = () => {
        if (!alive) return;
        setFailed(true);
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

  return (
    <div className="relative aspect-[9/16] w-24 shrink-0 overflow-hidden rounded-md border border-border bg-canvas sm:w-28">
      {device.online ? (
        <>
          {src && <img src={src} alt={`${device.name} screen`} className="h-full w-full object-contain" />}
          {!src && !failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
              <LoaderCircle size={20} className="animate-spin" />
              <span className="text-[11px]">Capturing</span>
            </div>
          )}
          {!src && failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-2 text-center text-ink-subtle">
              <ImageOff size={21} />
              <span className="text-[11px] leading-4">Capture unavailable</span>
            </div>
          )}
          <button
            type="button"
            onClick={() => setManualNonce((nonce) => nonce + 1)}
            className="absolute bottom-1.5 right-1.5 flex h-10 w-10 items-center justify-center rounded-md border border-white/10 bg-black/75 text-white transition-colors hover:bg-black focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            title="Refresh screenshot"
            aria-label={`Refresh screenshot for ${device.name}`}
          >
            <RefreshCw size={15} />
          </button>
        </>
      ) : (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
          <Smartphone size={25} strokeWidth={1.5} />
          <span className="text-[11px] font-medium">Offline</span>
        </div>
      )}
    </div>
  );
}

function outcomeStyle(outcome?: string): { dot: string; text: string; badge: string } {
  switch (outcome) {
    case "ok":
      return { dot: "bg-success", text: "text-success", badge: "bg-success-soft text-success" };
    case "timeout":
    case "error":
      return { dot: "bg-danger", text: "text-danger", badge: "bg-danger-soft text-danger" };
    case "device_gone":
    case "canceled":
      return { dot: "bg-warning", text: "text-warning", badge: "bg-warning-soft text-warning" };
    default:
      return { dot: "bg-ink-subtle", text: "text-ink-muted", badge: "bg-surface-hover text-ink-muted" };
  }
}

function eventLabel(event: DeviceEvent): string {
  if (event.kind === "connected") return "Device connected";
  if (event.kind === "disconnected") return `Device disconnected${event.detail ? `: ${event.detail}` : ""}`;
  const source = event.source ? `${event.source} · ` : "";
  const duration = event.duration_ms != null ? ` · ${event.duration_ms}ms` : "";
  const detail = event.outcome === "error" && event.detail ? `: ${event.detail}` : "";
  return `${source}${event.method}${duration}${detail}`;
}

function eventDot(event: DeviceEvent): string {
  if (event.kind === "connected") return "bg-success";
  if (event.kind === "disconnected") return "bg-warning";
  return outcomeStyle(event.outcome).dot;
}

function DeviceActivity({ device, onClose }: { device: DeviceView; onClose: () => void }) {
  const [events, setEvents] = useState<DeviceEvent[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout>;

    const load = async () => {
      try {
        const result = await api.deviceEvents(device.id);
        if (!alive) return;
        setEvents(result.events);
        setError(null);
      } catch (err) {
        if (alive) setError((err as Error).message);
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
    <Modal
      open
      onClose={onClose}
      title={`${device.name} activity`}
      description="Recent connections and commands. Updates automatically while open."
      className="sm:max-w-2xl"
    >
      <div className="mb-4 flex flex-wrap items-center gap-2 text-xs">
        <DeviceStatus online={device.online} />
        {!device.online && device.last_seen && <span className="text-ink-subtle">Last seen {relativeTime(device.last_seen)}</span>}
      </div>

      {error && (
        <div role="alert" className="mb-4 rounded-md border border-danger/25 bg-danger-soft px-3 py-2.5 text-sm text-danger">
          {error}
        </div>
      )}

      {events === null ? (
        <div className="space-y-2" aria-label="Loading activity">
          {[0, 1, 2, 3].map((item) => (
            <div key={item} className="skeleton h-14 rounded-md" />
          ))}
        </div>
      ) : events.length === 0 ? (
        <div className="rounded-md border border-dashed border-border-strong px-5 py-10 text-center">
          <Activity size={23} className="mx-auto text-ink-subtle" />
          <p className="mt-3 text-sm font-semibold text-ink">No activity yet</p>
          <p className="mx-auto mt-1 max-w-sm text-sm leading-6 text-ink-muted">
            Connection changes and agent commands will appear here.
          </p>
        </div>
      ) : (
        <ul className="divide-y divide-border overflow-hidden rounded-md border border-border">
          {events.map((event, index) => (
            <li key={`${event.ts}-${index}`} className="flex items-start gap-3 bg-canvas px-3.5 py-3">
              <span className={`mt-1.5 h-2 w-2 shrink-0 rounded-full ${eventDot(event)}`} aria-hidden="true" />
              <div className="min-w-0 flex-1">
                <p className={`break-words text-sm leading-5 ${event.kind === "command" ? outcomeStyle(event.outcome).text : "text-ink"}`}>
                  {eventLabel(event)}
                </p>
                <p className="mt-1 font-mono text-[11px] text-ink-subtle">{clockTime(event.ts)}</p>
              </div>
              {event.kind === "command" && (
                <span className={`shrink-0 rounded px-2 py-1 text-[10px] font-bold uppercase ${outcomeStyle(event.outcome).badge}`}>
                  {event.outcome ?? "pending"}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);
  const [activityId, setActivityId] = useState<string | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [newName, setNewName] = useState("My phone");
  const [renameDevice, setRenameDevice] = useState<DeviceView | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [rotateDevice, setRotateDevice] = useState<DeviceView | null>(null);
  const [removeDevice, setRemoveDevice] = useState<DeviceView | null>(null);
  const [busy, setBusy] = useState(false);
  const loadedOnce = useRef(false);

  const reload = async () => {
    try {
      setDevices(await api.devices());
      setError(null);
    } catch (err) {
      if (!loadedOnce.current) setError((err as Error).message);
    } finally {
      loadedOnce.current = true;
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
    const timer = setInterval(() => void reload(), DEVICES_POLL_MS);
    return () => clearInterval(timer);
  }, []);

  const runAction = async (action: () => Promise<void>) => {
    setBusy(true);
    setActionError(null);
    try {
      await action();
    } catch (err) {
      setActionError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const addDevice = async (event: React.FormEvent) => {
    event.preventDefault();
    await runAction(async () => {
      const created = await api.createDevice(newName.trim() || "New device");
      setAddOpen(false);
      setNewName("My phone");
      setReveal({
        title: `Connect ${created.name}`,
        wssUrl: deviceWsUrl(created.device_token),
        token: created.device_token,
      });
      await reload();
    });
  };

  const rename = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!renameDevice || !renameValue.trim()) return;
    await runAction(async () => {
      await api.renameDevice(renameDevice.id, renameValue.trim());
      setRenameDevice(null);
      await reload();
    });
  };

  const rotate = async () => {
    if (!rotateDevice) return;
    await runAction(async () => {
      const result = await api.rotateDeviceToken(rotateDevice.id);
      setRotateDevice(null);
      setReveal({
        title: `New token for ${rotateDevice.name}`,
        wssUrl: deviceWsUrl(result.device_token),
        token: result.device_token,
      });
    });
  };

  const remove = async () => {
    if (!removeDevice) return;
    await runAction(async () => {
      await api.deleteDevice(removeDevice.id);
      setRemoveDevice(null);
      await reload();
    });
  };

  const onlineCount = devices.filter((device) => device.online).length;

  return (
    <div>
      <header className="mb-7 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-xs font-semibold uppercase text-brand">Device workspace</p>
          <h1 className="mt-2 text-2xl font-semibold text-ink sm:text-[28px]">Connected devices</h1>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">
            Monitor live screens, inspect command activity, and manage connection credentials.
          </p>
        </div>
        <Button onClick={() => setAddOpen(true)}>
          <Plus size={17} />
          Add device
        </Button>
      </header>

      {!loading && !error && devices.length > 0 && (
        <div className="mb-5 flex flex-wrap items-center gap-x-5 gap-y-2 border-y border-border py-3 text-sm">
          <span className="flex items-center gap-2 text-ink-muted">
            <Wifi size={16} className="text-success" />
            <strong className="font-semibold text-ink">{onlineCount}</strong> online
          </span>
          <span className="flex items-center gap-2 text-ink-muted">
            <WifiOff size={16} />
            <strong className="font-semibold text-ink">{devices.length - onlineCount}</strong> offline
          </span>
          <span className="text-xs text-ink-subtle">Status refreshes every 5 seconds</span>
        </div>
      )}

      {actionError && (
        <div role="alert" className="mb-5 flex items-center justify-between gap-3 rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
          <span>{actionError}</span>
          <button type="button" onClick={() => setActionError(null)} className="min-h-10 shrink-0 font-semibold underline underline-offset-4">
            Dismiss
          </button>
        </div>
      )}

      {loading ? (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3" aria-label="Loading devices">
          {[0, 1, 2].map((item) => (
            <div key={item} className="flex gap-4 rounded-lg border border-border bg-surface p-3">
              <div className="skeleton aspect-[9/16] w-24 shrink-0 rounded-md sm:w-28" />
              <div className="flex flex-1 flex-col py-2">
                <div className="skeleton h-5 w-28 rounded" />
                <div className="skeleton mt-3 h-4 w-36 rounded" />
                <div className="skeleton mt-2 h-4 w-24 rounded" />
                <div className="skeleton mt-auto h-10 w-full rounded-md" />
              </div>
            </div>
          ))}
        </div>
      ) : error ? (
        <Card className="border-danger/25 p-6 text-center">
          <p className="text-sm font-semibold text-danger">Unable to load devices</p>
          <p className="mt-1 text-sm text-ink-muted">{error}</p>
          <Button variant="outline" className="mt-5" onClick={() => void reload()}>
            <RefreshCw size={16} />
            Try again
          </Button>
        </Card>
      ) : devices.length === 0 ? (
        <section className="rounded-lg border border-dashed border-border-strong bg-sidebar px-5 py-14 text-center sm:py-20">
          <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md bg-brand-soft text-brand">
            <Smartphone size={23} />
          </span>
          <h2 className="mt-4 text-base font-semibold text-ink">Pair your first device</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
            Create a device credential, then scan the QR code or paste its connection URL into the Abacad app.
          </p>
          <Button className="mt-6" onClick={() => setAddOpen(true)}>
            <Plus size={17} />
            Add device
          </Button>
        </section>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {devices.map((device) => (
            <DeviceCard
              key={device.id}
              device={device}
              onActivity={() => setActivityId(device.id)}
              onRename={() => {
                setRenameDevice(device);
                setRenameValue(device.name);
              }}
              onRotate={() => setRotateDevice(device)}
              onRemove={() => setRemoveDevice(device)}
            />
          ))}
        </div>
      )}

      <Modal
        open={addOpen}
        onClose={() => setAddOpen(false)}
        title="Add a device"
        description="Create a named connection credential for a phone or machine."
      >
        <form onSubmit={addDevice}>
          <div className="flex flex-col gap-2">
            <Label htmlFor="device-name">Device name</Label>
            <Input
              id="device-name"
              autoFocus
              required
              value={newName}
              onChange={(event) => setNewName(event.target.value)}
              placeholder="My phone"
            />
            <p className="text-xs text-ink-subtle">Use a name that makes the device easy to identify in agent commands.</p>
          </div>
          <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="ghost" onClick={() => setAddOpen(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !newName.trim()}>
              {busy && <LoaderCircle size={16} className="animate-spin" />}
              Create device
            </Button>
          </div>
        </form>
      </Modal>

      <Modal open={renameDevice !== null} onClose={() => setRenameDevice(null)} title="Rename device">
        <form onSubmit={rename}>
          <div className="flex flex-col gap-2">
            <Label htmlFor="rename-device">Device name</Label>
            <Input
              id="rename-device"
              autoFocus
              required
              value={renameValue}
              onChange={(event) => setRenameValue(event.target.value)}
            />
          </div>
          <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="ghost" onClick={() => setRenameDevice(null)}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !renameValue.trim()}>
              {busy && <LoaderCircle size={16} className="animate-spin" />}
              Save name
            </Button>
          </div>
        </form>
      </Modal>

      <Modal
        open={rotateDevice !== null}
        onClose={() => setRotateDevice(null)}
        title="Rotate device token?"
        description={`The current credential for ${rotateDevice?.name ?? "this device"} will stop working immediately.`}
      >
        <p className="text-sm leading-6 text-ink-muted">
          The device will disconnect and must be configured with the new connection URL before it can come online again.
        </p>
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="ghost" onClick={() => setRotateDevice(null)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void rotate()} disabled={busy}>
            {busy && <LoaderCircle size={16} className="animate-spin" />}
            Rotate token
          </Button>
        </div>
      </Modal>

      <Modal
        open={removeDevice !== null}
        onClose={() => setRemoveDevice(null)}
        title="Remove device?"
        description={`${removeDevice?.name ?? "This device"} will lose access to the workspace.`}
      >
        <p className="text-sm leading-6 text-ink-muted">
          This removes its credential and activity from the dashboard. The action cannot be undone.
        </p>
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="ghost" onClick={() => setRemoveDevice(null)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void remove()} disabled={busy}>
            {busy ? <LoaderCircle size={16} className="animate-spin" /> : <Trash2 size={16} />}
            Remove device
          </Button>
        </div>
      </Modal>

      <Modal
        open={reveal !== null}
        onClose={() => setReveal(null)}
        title={reveal?.title ?? ""}
        description="This credential is shown once. Connect the device before closing or store it securely."
        className="sm:max-w-2xl"
      >
        {reveal && (
          <div className="grid gap-6 sm:grid-cols-[200px_minmax(0,1fr)]">
            <div className="flex items-center justify-center rounded-md bg-white p-4">
              <QRCodeSVG value={reveal.wssUrl} size={168} title="Device connection QR code" />
            </div>
            <div className="min-w-0 space-y-4">
              <div>
                <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">Connection URL</p>
                <CopyField value={reveal.wssUrl} />
              </div>
              <div>
                <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">Device token</p>
                <CopyField value={reveal.token} />
              </div>
              <div className="flex items-start gap-2.5 border-t border-border pt-4 text-xs leading-5 text-ink-subtle">
                <ShieldCheck size={16} className="mt-0.5 shrink-0 text-brand" />
                The token grants device access. Keep it out of source control and shared logs.
              </div>
            </div>
            <div className="flex justify-end border-t border-border pt-5 sm:col-span-2">
              <Button onClick={() => setReveal(null)}>
                <CheckCircle2 size={17} />
                Device configured
              </Button>
            </div>
          </div>
        )}
      </Modal>

      {activityId &&
        (() => {
          const device = devices.find((item) => item.id === activityId);
          return device ? <DeviceActivity device={device} onClose={() => setActivityId(null)} /> : null;
        })()}
    </div>
  );
}

function DeviceCard({
  device,
  onActivity,
  onRename,
  onRotate,
  onRemove,
}: {
  device: DeviceView;
  onActivity: () => void;
  onRename: () => void;
  onRotate: () => void;
  onRemove: () => void;
}) {
  return (
    <Card className="flex min-w-0 flex-col p-3">
      <div className="flex min-w-0 gap-4">
        <DeviceScreenshot device={device} />
        <div className="flex min-w-0 flex-1 flex-col py-1">
          <DeviceStatus online={device.online} />
          <h2 className="mt-3 break-words text-base font-semibold leading-5 text-ink">{device.name}</h2>
          <div className="mt-3 space-y-2 text-xs leading-5 text-ink-muted">
            <p className="flex items-start gap-2">
              <Clock3 size={14} className="mt-0.5 shrink-0 text-ink-subtle" />
              <span>
                {device.online
                  ? "Connected now"
                  : device.last_seen
                    ? `Last seen ${relativeTime(device.last_seen)}`
                    : "Never connected"}
              </span>
            </p>
            <p className="text-ink-subtle">Added {formatDate(device.created_at)}</p>
          </div>
          <div className="mt-auto pt-4">
            <span className="font-mono text-[10px] text-ink-subtle" title={device.id}>
              {device.id.slice(0, 12)}
            </span>
          </div>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-[minmax(0,1fr)_44px_44px_44px] gap-2 border-t border-border pt-3">
        <Button variant="outline" size="sm" onClick={onActivity}>
          <Activity size={15} />
          Activity
        </Button>
        <Button variant="ghost" size="icon" onClick={onRename} title="Rename device" aria-label={`Rename ${device.name}`}>
          <Pencil size={16} />
        </Button>
        <Button variant="ghost" size="icon" onClick={onRotate} title="Rotate token" aria-label={`Rotate token for ${device.name}`}>
          <RefreshCw size={16} />
        </Button>
        <Button variant="ghost" size="icon" onClick={onRemove} title="Remove device" aria-label={`Remove ${device.name}`} className="hover:bg-danger-soft hover:text-danger">
          <Trash2 size={16} />
        </Button>
      </div>
    </Card>
  );
}

function DeviceStatus({ online }: { online: boolean }) {
  return (
    <span className={`inline-flex h-7 w-fit items-center gap-2 rounded-full border px-2.5 text-xs font-semibold ${online ? "border-success/20 bg-success-soft text-success" : "border-border bg-surface-raised text-ink-muted"}`}>
      {online ? <Wifi size={13} /> : <WifiOff size={13} />}
      {online ? "Online" : "Offline"}
    </span>
  );
}

function formatDate(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "recently";
  return date.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}
