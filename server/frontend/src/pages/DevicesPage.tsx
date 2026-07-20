import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { QRCodeSVG } from "qrcode.react";
import {
  CheckCircle2,
  Globe,
  LoaderCircle,
  Plus,
  RefreshCw,
  ShieldCheck,
  Smartphone,
} from "lucide-react";
import { api, type DeviceView } from "@/lib/api";
import { groupDevices, type FormFactor } from "@/lib/devices";
import { DeviceFrame, DeviceScreen } from "@/components/DeviceScreen";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";
import { PageHeader } from "@/components/PageHeader";

const DEVICES_POLL_MS = 5000;

interface Reveal {
  title: string;
  wssUrl: string;
  token: string;
  browserUrl?: string; // set for browser devices: the /b#<token> page to open
}

function deviceWsUrl(token: string): string {
  const scheme = window.location.protocol === "https:" ? "wss" : "ws";
  return `${scheme}://${window.location.host}/device?token=${token}`;
}

export function DevicesPage() {
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [reveal, setReveal] = useState<Reveal | null>(null);
  const [addOpen, setAddOpen] = useState(false);
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
        wssUrl: deviceWsUrl(created.device_token),
        token: created.device_token,
        browserUrl:
          platform === "browser"
            ? `${window.location.origin}/b#${created.device_token}`
            : undefined,
      });
      await reload();
    });
  };

  return (
    <div>
      <PageHeader
        eyebrow="console / devices"
        title="Devices"
        description="Every phone, machine, and browser you've paired. Open one to view its screen and live activity."
        actions={
          <Button onClick={() => setAddOpen(true)}>
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
          <div className="mt-4 flex flex-col gap-2">
            <Label htmlFor="device-type">Device type</Label>
            <select
              id="device-type"
              value={platform}
              onChange={(event) => setPlatform(event.target.value)}
              className="min-h-10 rounded-md border border-border bg-surface px-3 text-sm text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
            >
              <option value="android">Phone (Android app)</option>
              <option value="macos">Desktop (macOS app)</option>
              <option value="browser">Browser tab (no install)</option>
            </select>
            <p className="text-xs text-ink-subtle">
              {platform === "browser"
                ? "You'll get a link to open in any browser — the tab itself becomes the device the agent drives."
                : "Install the abacad app on the device, then scan the QR to connect it."}
            </p>
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
        <DeviceScreen device={device} factor={factor} onAspect={setAspect} onShot={setHasShot} />
      </DeviceFrame>
      <h3
        className="max-w-full truncate text-center font-display text-sm font-bold leading-tight text-ink transition-colors group-hover:text-brand"
        title={device.name}
      >
        {device.name}
      </h3>
    </Link>
  );
}
