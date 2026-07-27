import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { QRCodeSVG } from "qrcode.react";
import {
  CheckCircle2,
  Download,
  Globe,
  KeyRound,
  LoaderCircle,
  Monitor,
  Plus,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Terminal,
} from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { groupDevices, platformInfo, NEW_DEVICE_PLATFORMS, type FormFactor } from "@/lib/devices";
import { cn, untilTime } from "@/lib/utils";
import { DeviceFrame, DeviceScreen, ScreenPlaceholder } from "@/components/DeviceScreen";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";
import { PageHeader } from "@/components/PageHeader";

const DEVICES_POLL_MS = 5000;

function platformIcon(platform: string, factor: FormFactor) {
  if (platform === "browser") return Globe;
  return factor === "handset" ? Smartphone : Monitor;
}

function PlatformTile({
  platform,
  selected,
  onSelect,
}: {
  platform: string;
  selected: boolean;
  onSelect: () => void;
}) {
  const { label, factor } = platformInfo(platform);
  const Icon = platformIcon(platform, factor);
  return (
    <button
      type="button"
      role="radio"
      aria-checked={selected}
      onClick={onSelect}
      className="group flex min-w-0 flex-col gap-3 rounded-[1.4rem] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-4 focus-visible:ring-offset-surface-raised"
    >
      <span className="flex h-[168px] w-full items-center justify-center">
        <DeviceFrame
          factor={factor}
          aspect={null}
          maxWidth={factor === "handset" ? "max-w-[80px]" : "max-w-[210px]"}
          // These frames are ~half the size of the grid's, so the radius scales
          // down with them — the default would round a 80px-wide phone to a pill.
          className={cn(
            factor === "handset" ? "rounded-[12px]" : "rounded-[8px]",
            selected && "border-brand ring-1 ring-brand",
          )}
        >
          <ScreenPlaceholder icon={Icon} factor={factor} />
        </DeviceFrame>
      </span>
      <span
        className={cn(
          "max-w-full truncate text-center font-display text-sm font-bold leading-tight transition-colors group-hover:text-brand",
          selected ? "text-brand" : "text-ink",
        )}
      >
        {label}
      </span>
    </button>
  );
}

interface Reveal {
  title: string;
  wssUrl: string;
  token: string;
  browserUrl?: string; // set for browser devices: the <id>.abacad.ai page to open
}

// How the "Add device" modal starts: pick a route, or fall through to the manual
// token form that used to be the only option.
type AddMode = "choose" | "manual";

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);
  const [addOpen, setAddOpen] = useState(false);
  const [addMode, setAddMode] = useState<AddMode>("choose");
  const [newName, setNewName] = useState("My phone");
  const [platform, setPlatform] = useState("android");
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
      const created = await api.createDevice(newName.trim() || "New device", platform);
      setAddOpen(false);
      setNewName("My phone");
      setReveal({
        title: `Connect ${created.name}`,
        // Use the server's own wss_url rather than rebuilding it here. Deriving
        // it client-side silently diverges the moment the server changes shape,
        // and only this path would break — /pair already returns the real URL.
        wssUrl: created.wss_url,
        token: created.device_token,
        browserUrl: created.browser_url, // server sets this only for browser devices
      });
      await reload();
    });
  };

  return (
    <div>
      <PageHeader
        title="Devices"
        actions={
          <Button onClick={() => { setAddMode("choose"); setAddOpen(true); }}>
            <Plus size={17} />
            Add device
          </Button>
        }
      />

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
          <h2 className="mt-4 font-display text-lg font-bold text-ink">Add your first device</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
            Install abacad on the device and open it. It shows an ID and a claim code — type them here
            and it's yours.
          </p>
          <Button className="mt-6" onClick={() => { setAddMode("choose"); setAddOpen(true); }}>
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
        className="sm:max-w-3xl"
      >
        {addMode === "choose" ? (
          // Install-and-claim is the primary path now: the device shows its own
          // id and claim code, so nothing secret is copied toward it. Minting a
          // token here and carrying it to the device is the older, weaker shape
          // — kept for browser devices and self-hosters, demoted to "Advanced".
          <div className="flex flex-col gap-3">
            <a
              href="/downloads"
              className="flex items-start gap-4 rounded-md border border-brand/30 bg-brand-soft/40 p-4 text-left transition-colors hover:bg-brand-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-surface text-brand">
                <Download size={18} />
              </span>
              <span className="min-w-0">
                <span className="block font-display text-sm font-bold text-ink">Install the app</span>
                <span className="mt-1 block text-sm leading-6 text-ink-muted">
                  Open abacad on the device — it shows an ID and a claim code you type here. Nothing to
                  copy onto the device.
                </span>
              </span>
            </a>
            <a
              href="/claim"
              className="flex items-start gap-4 rounded-md border border-border p-4 text-left transition-colors hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border-strong bg-surface text-ink-muted">
                <KeyRound size={18} />
              </span>
              <span className="min-w-0">
                <span className="block font-display text-sm font-bold text-ink">I have a claim code</span>
                <span className="mt-1 block text-sm leading-6 text-ink-muted">
                  The device is already showing its ID and code.
                </span>
              </span>
            </a>
            <button
              type="button"
              onClick={() => setAddMode("manual")}
              className="flex items-start gap-4 rounded-md border border-border p-4 text-left transition-colors hover:bg-surface-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              <span className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border-strong bg-surface text-ink-muted">
                <Terminal size={18} />
              </span>
              <span className="min-w-0">
                <span className="block font-display text-sm font-bold text-ink">
                  Advanced: create a credential
                </span>
                <span className="mt-1 block text-sm leading-6 text-ink-muted">
                  Mint a token and carry it to the device yourself. Needed for browser devices, and for
                  clients that can't reach this server.
                </span>
              </span>
            </button>
          </div>
        ) : (
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
          </div>
          <fieldset className="mt-6 flex flex-col gap-3">
            <legend className="mb-3 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">
              Device type
            </legend>
            <div role="radiogroup" className="grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(160px,1fr))]">
              {NEW_DEVICE_PLATFORMS.map((value) => (
                <PlatformTile
                  key={value}
                  platform={value}
                  selected={platform === value}
                  onSelect={() => setPlatform(value)}
                />
              ))}
            </div>
          </fieldset>
          <div className="mt-8 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <Button type="button" variant="ghost" onClick={() => setAddMode("choose")}>
              Back
            </Button>
            <Button type="submit" disabled={busy || !newName.trim()}>
              {busy && <LoaderCircle size={16} className="animate-spin" />}
              Create device
            </Button>
          </div>
        </form>
        )}
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
              <QRCodeSVG
                value={reveal.browserUrl ?? reveal.wssUrl}
                size={168}
                title={reveal.browserUrl ? "Open browser device QR code" : "Device connection QR code"}
              />
            </div>
            <div className="min-w-0 space-y-4">
              <div>
                <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">
                  {reveal.browserUrl ? "Open on the device" : "Connection URL"}
                </p>
                <CopyField value={reveal.browserUrl ?? reveal.wssUrl} />
                {reveal.browserUrl && (
                  <a
                    href={reveal.browserUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="mt-2 inline-flex items-center gap-1.5 text-sm font-semibold text-brand underline underline-offset-4"
                  >
                    <Globe size={15} />
                    Open in a new tab
                  </a>
                )}
              </div>
              <div>
                <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Device token</p>
                <CopyField value={reveal.token} />
              </div>
              <div className="flex items-start gap-2.5 border-t border-border pt-4 text-xs leading-5 text-ink-subtle">
                <ShieldCheck size={16} className="mt-0.5 shrink-0 text-brand" />
                {reveal.browserUrl
                  ? "This link embeds the device token. Open it on the screen you want to control (phone, TV, laptop) — or scan the QR to open it there — and keep the link private."
                  : "The token grants device access. Keep it out of source control and shared logs."}
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
  const [hasShot, setHasShot] = useState(false);

  return (
    <Link
      to={`/devices/${device.id}`}
      className="group flex min-w-0 flex-col gap-3 rounded-[1.4rem] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-4 focus-visible:ring-offset-canvas"
    >
      <DeviceFrame factor={factor} aspect={aspect} bare={hasShot}>
        <DeviceScreen device={device} factor={factor} onAspect={setAspect} onShot={setHasShot} pauseWhenAsleep />
      </DeviceFrame>
      <h3
        className="max-w-full truncate text-center font-display text-sm font-bold leading-tight text-ink transition-colors group-hover:text-brand"
        title={device.name}
      >
        {device.name}
      </h3>
      {device.expires_at && (
        <p className="-mt-2 text-center text-[11px] font-medium text-ink-subtle">
          expires {untilTime(device.expires_at)}
        </p>
      )}
    </Link>
  );
}
