import { useEffect, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  Activity,
  CheckCircle2,
  ImageOff,
  LoaderCircle,
  Monitor,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Trash2,
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

type FormFactor = "handset" | "desktop";

// The live screen — an absolutely-positioned layer that fills the frame's
// screen cutout. The screenshot bleeds edge-to-edge (object-cover), so the
// card reads as the device itself rather than a thumbnail in a box.
function DeviceScreen({ device, factor }: { device: DeviceView; factor: FormFactor }) {
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

  const OfflineIcon = factor === "handset" ? Smartphone : Monitor;

  return (
    <>
      {device.online ? (
        <>
          {src && (
            <img
              src={src}
              alt={`${device.name} screen`}
              className="absolute inset-0 h-full w-full object-cover object-top"
            />
          )}
          {!src && !failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 text-ink-subtle">
              <LoaderCircle size={20} className="animate-spin" />
              <span className="font-mono text-[10px] uppercase tracking-wider">Capturing</span>
            </div>
          )}
          {!src && failed && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 px-3 text-center text-ink-subtle">
              <ImageOff size={22} />
              <span className="font-mono text-[10px] uppercase leading-4 tracking-wider">No capture</span>
            </div>
          )}
          <button
            type="button"
            onClick={() => setManualNonce((nonce) => nonce + 1)}
            className="absolute bottom-2 right-2 z-20 flex h-8 w-8 items-center justify-center rounded-full border border-white/10 bg-black/55 text-white backdrop-blur transition-colors hover:bg-black/85 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            title="Refresh screenshot"
            aria-label={`Refresh screenshot for ${device.name}`}
          >
            <RefreshCw size={13} />
          </button>
        </>
      ) : (
        <div className="absolute inset-0 flex flex-col items-center justify-center gap-1.5 text-ink-subtle">
          <OfflineIcon size={factor === "handset" ? 24 : 30} strokeWidth={1.25} />
          <span className="font-mono text-[10px] uppercase tracking-[0.22em]">Signal lost</span>
          {device.last_seen && (
            <span className="font-mono text-[10px]">seen {relativeTime(device.last_seen)}</span>
          )}
        </div>
      )}

      <span
        className={`absolute z-20 inline-flex items-center gap-1.5 rounded-full px-2 py-1 font-mono text-[10px] font-medium uppercase tracking-wider backdrop-blur ${
          factor === "handset" ? "bottom-2 left-2" : "left-2 top-2"
        } ${
          device.online
            ? "bg-black/50 text-[#4ade80] ring-1 ring-[#4ade80]/30"
            : "bg-surface/85 text-ink-muted ring-1 ring-border"
        }`}
      >
        <span className={`h-1.5 w-1.5 rounded-full ${device.online ? "pulse-dot bg-[#4ade80]" : "bg-ink-subtle"}`} />
        {device.online ? "online" : "offline"}
      </span>
    </>
  );
}

// A lightweight frame: just a hairline border and a soft shadow. Form factor
// shows through the aspect ratio and corner radius — very rounded for a phone,
// gently rounded for a screen — while the capture bleeds to the edge.
function DeviceFrame({ factor, children }: { factor: FormFactor; children: React.ReactNode }) {
  const shape = factor === "handset" ? "aspect-[9/18.5] rounded-[1.7rem]" : "aspect-[16/10] rounded-[12px]";
  return (
    <div className={`mx-auto w-full ${factor === "handset" ? "max-w-[176px]" : ""}`}>
      <div
        className={`relative overflow-hidden border border-border bg-surface-raised shadow-[0_10px_24px_-16px_var(--shadow-strong)] transition-transform duration-200 hover:-translate-y-0.5 ${shape}`}
      >
        {children}
      </div>
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
        {!device.online && device.last_seen && (
          <span className="font-mono text-[11px] text-ink-subtle">last seen {relativeTime(device.last_seen)}</span>
        )}
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
                <span className={`shrink-0 rounded px-2 py-1 font-mono text-[10px] font-bold uppercase ${outcomeStyle(event.outcome).badge}`}>
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

  return (
    <div>
      <div className="mb-7 flex justify-end">
        <Button onClick={() => setAddOpen(true)}>
          <Plus size={17} />
          Add device
        </Button>
      </div>

      {actionError && (
        <div role="alert" className="mb-5 flex items-center justify-between gap-3 rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
          <span>{actionError}</span>
          <button type="button" onClick={() => setActionError(null)} className="min-h-10 shrink-0 font-semibold underline underline-offset-4">
            Dismiss
          </button>
        </div>
      )}

      {loading ? (
        <div
          className="grid gap-x-5 gap-y-7 [grid-template-columns:repeat(auto-fill,minmax(300px,1fr))]"
          aria-label="Loading devices"
        >
          {[0, 1, 2].map((item) => (
            <div key={item} className="flex flex-col gap-3">
              <div className="skeleton aspect-[16/10] rounded-[12px]" />
              <div className="skeleton h-4 w-28 rounded" />
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
        <section className="rounded-[10px] border border-dashed border-border-strong bg-surface px-5 py-14 text-center sm:py-20">
          <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
            <Smartphone size={23} />
          </span>
          <h2 className="mt-4 font-display text-lg font-bold text-ink">Pair your first device</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
            Create a device credential, then scan the QR code or paste its connection URL into the Abacad app.
          </p>
          <Button className="mt-6" onClick={() => setAddOpen(true)}>
            <Plus size={17} />
            Add device
          </Button>
        </section>
      ) : (
        <div className="space-y-10">
          {groupDevices(devices).map((group) => (
            <section key={group.key}>
              <h2 className="mb-4 font-display text-[13px] font-bold uppercase tracking-[0.16em] text-ink-muted">
                {group.label}
              </h2>
              <div
                className={
                  group.factor === "handset"
                    ? "grid gap-x-4 gap-y-6 [grid-template-columns:repeat(auto-fill,minmax(148px,1fr))]"
                    : "grid gap-x-5 gap-y-7 [grid-template-columns:repeat(auto-fill,minmax(300px,1fr))]"
                }
              >
                {group.devices.map((device) => (
                  <DeviceCard
                    key={device.id}
                    device={device}
                    factor={group.factor}
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
            </section>
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
                <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Connection URL</p>
                <CopyField value={reveal.wssUrl} />
              </div>
              <div>
                <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Device token</p>
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
  factor,
  onActivity,
  onRename,
  onRotate,
  onRemove,
}: {
  device: DeviceView;
  factor: FormFactor;
  onActivity: () => void;
  onRename: () => void;
  onRotate: () => void;
  onRemove: () => void;
}) {
  const actions = (
    <div className="flex shrink-0 items-center gap-0.5">
      <IconAction icon={Activity} tip="Activity" aria={`View activity for ${device.name}`} onClick={onActivity} />
      <IconAction icon={Pencil} tip="Rename" aria={`Rename ${device.name}`} onClick={onRename} />
      <IconAction icon={RefreshCw} tip="Rotate token" aria={`Rotate token for ${device.name}`} onClick={onRotate} />
      <IconAction icon={Trash2} tip="Remove" aria={`Remove ${device.name}`} onClick={onRemove} danger />
    </div>
  );

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <DeviceFrame factor={factor}>
        <DeviceScreen device={device} factor={factor} />
      </DeviceFrame>

      {/* Narrow phone cards stack the name over the actions so it isn't crushed;
          wider desktop cards keep name and actions on one line. */}
      {factor === "handset" ? (
        <div className="flex min-w-0 flex-col items-center gap-1.5">
          <h3 className="max-w-full truncate text-center font-display text-sm font-bold leading-tight text-ink" title={device.name}>
            {device.name}
          </h3>
          {actions}
        </div>
      ) : (
        <div className="flex min-w-0 items-center gap-0.5 px-0.5">
          <h3 className="min-w-0 flex-1 truncate font-display text-sm font-bold leading-tight text-ink" title={device.name}>
            {device.name}
          </h3>
          {actions}
        </div>
      )}
    </div>
  );
}

function IconAction({
  icon: Icon,
  tip,
  aria,
  onClick,
  danger,
}: {
  icon: typeof Activity;
  tip: string;
  aria: string;
  onClick: () => void;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={tip}
      aria-label={aria}
      className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-ink-subtle transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
        danger ? "hover:text-danger" : "hover:text-ink"
      }`}
    >
      <Icon size={15} />
    </button>
  );
}

function DeviceStatus({ online }: { online: boolean }) {
  return (
    <span
      className={`inline-flex h-7 w-fit items-center gap-2 rounded-full border px-2.5 font-mono text-[11px] font-medium uppercase tracking-wider ${
        online
          ? "border-success/25 bg-success-soft text-success"
          : "border-border bg-surface-raised text-ink-muted"
      }`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${online ? "pulse-dot bg-success" : "bg-ink-subtle"}`} />
      {online ? "online" : "offline"}
    </span>
  );
}

// --- platform grouping ---
//
// Devices carry a platform string (e.g. "android", "macos"). It can be blank on
// older devices, so we fall back to inferring from the name. Everything maps to
// a display label and a form factor, which drives both the section a device
// lands in and the frame it wears.

interface PlatformInfo {
  label: string;
  factor: FormFactor;
}

interface PlatformGroup extends PlatformInfo {
  key: string;
  devices: DeviceView[];
}

const KNOWN_PLATFORMS: Record<string, PlatformInfo> = {
  macos: { label: "macOS", factor: "desktop" },
  mac: { label: "macOS", factor: "desktop" },
  darwin: { label: "macOS", factor: "desktop" },
  osx: { label: "macOS", factor: "desktop" },
  windows: { label: "Windows", factor: "desktop" },
  win32: { label: "Windows", factor: "desktop" },
  linux: { label: "Linux", factor: "desktop" },
  android: { label: "Android", factor: "handset" },
  ios: { label: "iOS", factor: "handset" },
  ipados: { label: "iPadOS", factor: "handset" },
};

// Section order — desktops first, then handsets, with unrecognized labels last.
const GROUP_ORDER = ["macOS", "Windows", "Linux", "Desktop", "iPadOS", "iOS", "Android", "Mobile", "Other"];

function classifyText(text: string): PlatformInfo | null {
  const t = text.toLowerCase();
  if (/macbook|imac|mac ?mini|mac ?studio|\bmac\b|macos|osx|darwin/.test(t)) return { label: "macOS", factor: "desktop" };
  if (/windows|\bwin\b|\bpc\b|thinkpad|surface/.test(t)) return { label: "Windows", factor: "desktop" };
  if (/linux|ubuntu|debian|fedora|arch/.test(t)) return { label: "Linux", factor: "desktop" };
  if (/iphone|ipad|\bios\b/.test(t)) return { label: "iOS", factor: "handset" };
  if (/android|pixel|galaxy|samsung|\bzte\b|xiaomi|redmi|oneplus|oppo|vivo|nexus|moto|huawei|honor|nokia/.test(t))
    return { label: "Android", factor: "handset" };
  if (/phone|mobile|tablet/.test(t)) return { label: "Mobile", factor: "handset" };
  if (/desktop|laptop|computer/.test(t)) return { label: "Desktop", factor: "desktop" };
  return null;
}

function resolvePlatform(device: DeviceView): PlatformInfo {
  const p = (device.platform ?? "").trim().toLowerCase();
  return (
    (p ? KNOWN_PLATFORMS[p] ?? classifyText(p) : null) ??
    classifyText(device.name) ?? { label: "Other", factor: "desktop" }
  );
}

function groupDevices(devices: DeviceView[]): PlatformGroup[] {
  const groups = new Map<string, PlatformGroup>();
  for (const device of devices) {
    const info = resolvePlatform(device);
    const key = info.label.toLowerCase();
    let group = groups.get(key);
    if (!group) {
      group = { key, label: info.label, factor: info.factor, devices: [] };
      groups.set(key, group);
    }
    group.devices.push(device);
  }

  const rank = (label: string) => {
    const index = GROUP_ORDER.indexOf(label);
    return index === -1 ? GROUP_ORDER.length : index;
  };

  return [...groups.values()]
    .map((group) => ({
      ...group,
      devices: [...group.devices].sort(
        (a, b) => Number(b.online) - Number(a.online) || a.name.localeCompare(b.name),
      ),
    }))
    .sort((a, b) => rank(a.label) - rank(b.label) || a.label.localeCompare(b.label));
}
