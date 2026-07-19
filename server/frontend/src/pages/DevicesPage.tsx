import { useEffect, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import {
  CheckCircle2,
  ImageOff,
  LoaderCircle,
  Monitor,
  Plus,
  RefreshCw,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { relativeTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

const DEVICES_POLL_MS = 5000;
const SCREENSHOT_GAP_MS = 2000;

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

// The live screen — an absolutely-positioned layer inside the frame. The frame
// is sized to the screenshot's own aspect ratio, so object-contain fills it
// exactly: the capture is shown whole, never cropped or stretched. On each load
// we report the image's natural aspect ratio up to the frame via onAspect.
function DeviceScreen({
  device,
  factor,
  onAspect,
}: {
  device: DeviceView;
  factor: FormFactor;
  onAspect: (ratio: number | null) => void;
}) {
  const [src, setSrc] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    if (!device.online) {
      setSrc(null);
      setFailed(false);
      onAspect(null);
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
        if (img.naturalWidth && img.naturalHeight) {
          onAspect(img.naturalWidth / img.naturalHeight);
        }
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
  }, [device.online, device.id, onAspect]);

  const OfflineIcon = factor === "handset" ? Smartphone : Monitor;

  return (
    <>
      {device.online ? (
        <>
          {src && (
            <img
              src={src}
              alt={`${device.name} screen`}
              className="absolute inset-0 h-full w-full object-contain"
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
    </>
  );
}

// A lightweight frame: a hairline border and a soft shadow. When a screenshot
// has loaded, the frame takes that image's exact aspect ratio, so the capture is
// shown at its true shape — never cropped, never stretched. Until then (loading
// or offline) it falls back to a device-shaped ratio: tall for a phone, wide for
// a screen. Corner radius still signals form factor — very rounded for a phone,
// gently rounded for a screen.
function DeviceFrame({
  factor,
  aspect,
  children,
}: {
  factor: FormFactor;
  aspect: number | null;
  children: React.ReactNode;
}) {
  const radius = factor === "handset" ? "rounded-[1.7rem]" : "rounded-[12px]";
  const ratio = aspect ?? (factor === "handset" ? 9 / 18.5 : 16 / 10);
  return (
    <div className={`mx-auto w-full ${factor === "handset" ? "max-w-[176px]" : ""}`}>
      <div
        className={`relative overflow-hidden border border-border bg-surface-raised shadow-[0_10px_24px_-16px_var(--shadow-strong)] transition-transform duration-200 hover:-translate-y-0.5 ${radius}`}
        style={{ aspectRatio: ratio }}
      >
        {children}
      </div>
    </div>
  );
}

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [newName, setNewName] = useState("My phone");
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
            Create a device credential, then scan the QR code or paste its connection URL into the abacad app.
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
                  <DeviceCard key={device.id} device={device} factor={group.factor} />
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
    </div>
  );
}

function DeviceCard({ device, factor }: { device: DeviceView; factor: FormFactor }) {
  const [aspect, setAspect] = useState<number | null>(null);

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <DeviceFrame factor={factor} aspect={aspect}>
        <DeviceScreen device={device} factor={factor} onAspect={setAspect} />
      </DeviceFrame>
      <h3
        className="max-w-full truncate text-center font-display text-sm font-bold leading-tight text-ink"
        title={device.name}
      >
        {device.name}
      </h3>
    </div>
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
