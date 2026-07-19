import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { ArrowLeft, Plug, RefreshCw, Smartphone, TerminalSquare, Unplug } from "lucide-react";
import { ApiError, api, type DeviceEvent, type DeviceView } from "@/lib/api";
import { clockTime, relativeTime } from "@/lib/utils";
import { resolvePlatform } from "@/lib/devices";
import { DeviceFrame, DeviceScreen } from "@/components/DeviceScreen";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/CopyField";

const DEVICE_POLL_MS = 5000;

export function DeviceDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [device, setDevice] = useState<DeviceView | null>(null);
  const [events, setEvents] = useState<DeviceEvent[] | null>(null);
  const [aspect, setAspect] = useState<number | null>(null);
  const [hasShot, setHasShot] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const loadedOnce = useRef(false);

  const load = useCallback(async () => {
    if (!id) return;
    try {
      setDevice(await api.device(id));
      setError(null);
      setNotFound(false);
    } catch (err) {
      if (err instanceof ApiError && err.status === 404) {
        setNotFound(true);
      } else if (!loadedOnce.current) {
        setError((err as Error).message);
      }
    } finally {
      loadedOnce.current = true;
    }
    try {
      setEvents((await api.deviceEvents(id)).events);
    } catch {
      /* keep the last-known events; the device row still loads on its own */
    }
  }, [id]);

  useEffect(() => {
    loadedOnce.current = false;
    setDevice(null);
    setEvents(null);
    setAspect(null);
    setNotFound(false);
    setError(null);
    void load();
    const timer = setInterval(() => void load(), DEVICE_POLL_MS);
    return () => clearInterval(timer);
  }, [load]);

  const platform = device ? resolvePlatform(device) : null;
  const factor = platform?.factor ?? "desktop";

  return (
    <div>
      <Link
        to="/"
        className="inline-flex h-10 items-center gap-1.5 text-[13px] font-semibold text-ink-muted transition-colors hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
      >
        <ArrowLeft size={16} />
        Devices
      </Link>

      {notFound ? (
        <Card className="mt-4 p-8 text-center">
          <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-border bg-canvas text-ink-muted">
            <Smartphone size={22} />
          </span>
          <h1 className="mt-4 font-display text-lg font-bold text-ink">Device not found</h1>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
            This device doesn't exist or belongs to another workspace.
          </p>
          <Link
            to="/"
            className="mt-6 inline-flex h-11 items-center justify-center rounded-md border border-border-strong bg-surface px-4 text-sm font-semibold text-ink transition-colors hover:border-ink-subtle hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
          >
            Back to devices
          </Link>
        </Card>
      ) : error ? (
        <Card className="mt-4 border-danger/25 p-6 text-center">
          <p className="text-sm font-semibold text-danger">Unable to load device</p>
          <p className="mt-1 text-sm text-ink-muted">{error}</p>
          <Button variant="outline" className="mt-5" onClick={() => void load()}>
            <RefreshCw size={16} />
            Try again
          </Button>
        </Card>
      ) : !device ? (
        <div className="mt-6 grid gap-8 lg:grid-cols-[minmax(0,1fr)_360px]" aria-label="Loading device">
          <div className="skeleton aspect-[16/10] w-full max-w-[640px] rounded-[12px]" />
          <div className="space-y-3">
            <div className="skeleton h-8 w-48 rounded" />
            <div className="skeleton h-40 w-full rounded-[10px]" />
          </div>
        </div>
      ) : (
        <>
          <header className="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2">
            <h1 className="min-w-0 truncate font-display text-2xl font-bold leading-tight text-ink sm:text-3xl" title={device.name}>
              {device.name}
            </h1>
            <StatusPill online={device.online} />
            <span className="rounded-full border border-border bg-surface px-2.5 py-1 font-mono text-[11px] font-medium uppercase tracking-wider text-ink-muted">
              {platform?.label}
            </span>
          </header>

          <div className="mt-6 grid gap-8 lg:grid-cols-[minmax(0,1fr)_360px]">
            <DeviceFrame
              factor={factor}
              aspect={aspect}
              bare={hasShot}
              maxWidth={factor === "handset" ? "max-w-[300px]" : "max-w-[640px]"}
            >
              <DeviceScreen device={device} factor={factor} onAspect={setAspect} onShot={setHasShot} />
            </DeviceFrame>

            <div className="space-y-6">
              <Card className="p-5 sm:p-6">
                <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">
                  Device ID
                </p>
                <CopyField value={device.id} />

                <dl className="mt-5">
                  <MetaRow label="Platform">{platform?.label}</MetaRow>
                  <MetaRow label="Last seen">
                    {device.last_seen ? relativeTime(device.last_seen) : device.online ? "now" : "—"}
                  </MetaRow>
                  <MetaRow label="Added">{relativeTime(device.created_at)}</MetaRow>
                </dl>

                {device.ssh_host && (
                  <div className="mt-5">
                    <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">
                      SSH
                    </p>
                    <CopyField value={`ssh ${device.ssh_host}`} />
                  </div>
                )}
              </Card>
            </div>
          </div>

          <section className="mt-10">
            <h2 className="mb-3 font-display text-[13px] font-bold uppercase tracking-[0.16em] text-ink-muted">
              Recent activity
            </h2>
            <EventLog events={events} />
          </section>
        </>
      )}
    </div>
  );
}

function StatusPill({ online }: { online: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-bold uppercase tracking-wider ${
        online ? "bg-success-soft text-success" : "bg-surface-hover text-ink-muted"
      }`}
    >
      <span className={`h-1.5 w-1.5 rounded-full ${online ? "animate-pulse bg-success" : "bg-ink-subtle"}`} />
      {online ? "Online" : "Offline"}
    </span>
  );
}

function MetaRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 border-b border-border py-2.5 last:border-0">
      <dt className="font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">{label}</dt>
      <dd className="min-w-0 truncate text-right text-sm text-ink">{children}</dd>
    </div>
  );
}

function eventText(e: DeviceEvent): string {
  switch (e.kind) {
    case "connected":
      return "Connected";
    case "disconnected":
      return `Disconnected${e.detail ? `: ${e.detail}` : ""}`;
    case "command":
      return `${e.method ?? "command"}${e.outcome === "error" && e.detail ? `: ${e.detail}` : ""}`;
    default:
      return e.kind;
  }
}

function eventIcon(kind: DeviceEvent["kind"]) {
  switch (kind) {
    case "connected":
      return Plug;
    case "disconnected":
      return Unplug;
    default:
      return TerminalSquare;
  }
}

function outcomeBadge(outcome?: string): string {
  switch (outcome) {
    case "ok":
      return "bg-success-soft text-success";
    case "timeout":
    case "error":
      return "bg-danger-soft text-danger";
    case "device_gone":
    case "canceled":
      return "bg-warning-soft text-warning";
    default:
      return "bg-surface-hover text-ink-muted";
  }
}

function EventLog({ events }: { events: DeviceEvent[] | null }) {
  if (events === null) {
    return (
      <div className="space-y-2" aria-label="Loading activity">
        {[0, 1, 2].map((i) => (
          <div key={i} className="skeleton h-12 rounded-md" />
        ))}
      </div>
    );
  }
  if (events.length === 0) {
    return (
      <Card className="px-5 py-10 text-center text-sm text-ink-muted">
        No recent activity for this device yet.
      </Card>
    );
  }
  return (
    <ul className="divide-y divide-border overflow-hidden rounded-[10px] border border-border bg-surface">
      {events.map((e, i) => {
        const Icon = eventIcon(e.kind);
        return (
          <li key={`${e.ts}-${i}`} className="flex items-start gap-3 px-3.5 py-3">
            <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-border bg-canvas text-ink-muted">
              <Icon size={14} />
            </span>
            <div className="min-w-0 flex-1">
              <p className="break-words text-sm leading-5 text-ink">{eventText(e)}</p>
              <p className="mt-1 font-mono text-[11px] text-ink-subtle">
                {clockTime(e.ts)}
                {e.source ? ` · ${e.source}` : ""}
                {e.duration_ms ? ` · ${e.duration_ms}ms` : ""}
              </p>
            </div>
            {e.kind === "command" && (
              <span
                className={`mt-1 shrink-0 rounded px-2 py-1 font-mono text-[10px] font-bold uppercase ${outcomeBadge(e.outcome)}`}
              >
                {e.outcome ?? "pending"}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
