import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Cable, KeyRound, Plug, RefreshCw, Smartphone, TerminalSquare, Unplug } from "lucide-react";
import { ApiError, api, type ActivityItem, type DeviceView } from "@/lib/api";
import { clockTime, relativeTime } from "@/lib/utils";
import { resolvePlatform, type PlatformInfo } from "@/lib/devices";
import { DeviceFrame, DeviceScreen } from "@/components/DeviceScreen";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/CopyField";

const DEVICE_POLL_MS = 5000;

export function DeviceDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [device, setDevice] = useState<DeviceView | null>(null);
  const [events, setEvents] = useState<ActivityItem[] | null>(null);
  const [aspect, setAspect] = useState<number | null>(null);
  const [hasShot, setHasShot] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [needsKey, setNeedsKey] = useState(false);
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
      // The device's full trail — every activity scoped to this device: commands,
      // connects/disconnects, SSH sessions, tunnels, and lifecycle. This is the
      // persistent account trail filtered by device, not the in-memory command log.
      setEvents((await api.activities({ device: id, limit: 50 })).activities);
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

  // Only nudge users who haven't registered a key yet; SSH keys change rarely,
  // so fetch once rather than on the device poll.
  useEffect(() => {
    void api
      .sshKeys()
      .then((k) => setNeedsKey(k.length === 0))
      .catch(() => {});
  }, []);

  const platform = device ? resolvePlatform(device) : null;
  const factor = platform?.factor ?? "desktop";

  if (notFound) {
    return (
      <Card className="p-8 text-center">
        <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-border bg-canvas text-ink-muted">
          <Smartphone size={22} />
        </span>
        <h1 className="mt-4 font-display text-lg font-bold text-ink">Device not found</h1>
        <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
          This device doesn't exist or belongs to another workspace.
        </p>
        <Link
          to="/devices"
          className="mt-6 inline-flex h-11 items-center justify-center rounded-md border border-border-strong bg-surface px-4 text-sm font-semibold text-ink transition-colors hover:border-ink-subtle hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-canvas"
        >
          Back to devices
        </Link>
      </Card>
    );
  }

  if (error) {
    return (
      <Card className="border-danger/25 p-6 text-center">
        <p className="text-sm font-semibold text-danger">Unable to load device</p>
        <p className="mt-1 text-sm text-ink-muted">{error}</p>
        <Button variant="outline" className="mt-5" onClick={() => void load()}>
          <RefreshCw size={16} />
          Try again
        </Button>
      </Card>
    );
  }

  if (!device) {
    return (
      <div aria-label="Loading device">
        <div className="skeleton aspect-[16/10] w-full rounded-[12px]" />
        <div className="mt-8 grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(260px,1fr))]">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="skeleton h-56 rounded-[10px]" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <>
      <header className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h1
          className="min-w-0 truncate font-display text-2xl font-bold leading-tight text-ink sm:text-3xl"
          title={device.name}
        >
          {device.name}
        </h1>
        <StatusPill online={device.online} />
        <span className="rounded-full border border-border bg-surface px-2.5 py-1 font-mono text-[11px] font-medium uppercase tracking-wider text-ink-muted">
          {platform?.label}
        </span>
      </header>

      {/* The screenshot is the hero — it spans the full content width. The frame
          sizes itself to the capture's aspect ratio; only handsets keep a cap so a
          tall portrait shot doesn't blow up the page. */}
      <div className="mt-6">
        <DeviceFrame
          factor={factor}
          aspect={aspect}
          bare={hasShot}
          maxWidth={factor === "handset" ? "max-w-[360px]" : ""}
        >
          <DeviceScreen device={device} factor={factor} onAspect={setAspect} onShot={setHasShot} />
        </DeviceFrame>
      </div>

      {/* Four guideline columns. auto-fit + minmax gives each a minimum width and
          wraps to fewer columns (2, then 1) as the viewport narrows; grid rows keep
          the cards on a row at equal height. */}
      <div className="mt-8 grid gap-4 [grid-template-columns:repeat(auto-fit,minmax(260px,1fr))]">
        <Column title="Guidelines">
          <p className="mb-4 text-sm leading-6 text-ink-muted">
            {factor === "handset" ? "A phone" : "A machine"} linked to your abacad account. Agents drive it by its
            device ID.
          </p>
          <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">
            Device ID
          </p>
          <CopyField value={device.id} />
          <dl className="mt-4">
            <MetaRow label="Platform">{platform?.label}</MetaRow>
            <MetaRow label="Last seen">
              {device.last_seen ? relativeTime(device.last_seen) : device.online ? "now" : "—"}
            </MetaRow>
            <MetaRow label="Added">{relativeTime(device.created_at)}</MetaRow>
          </dl>
        </Column>

        <Column title="Setup">
          <p className="text-sm leading-6 text-ink-muted">{setupText(platform)}</p>
        </Column>

        <Column title="Access">
          <AccessGuide device={device} needsKey={needsKey} />
        </Column>

        <Column title="Activities">
          <EventLog events={events} />
        </Column>
      </div>
    </>
  );
}

// A single guideline column: a titled card in the responsive 4-up row.
function Column({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card className="p-5">
      <h2 className="mb-4 font-display text-[13px] font-bold uppercase tracking-[0.16em] text-ink-muted">{title}</h2>
      {children}
    </Card>
  );
}

// Platform-appropriate one-liner on how this device connects to abacad. Wording
// mirrors the add-device flow (DevicesPage): browser devices are zero-install, an
// app-backed device connects with its device token.
function setupText(platform: PlatformInfo | null): string {
  if (platform?.label === "Browser") {
    return "Open the device link in any browser tab — the tab itself becomes the device your agent drives. No install needed.";
  }
  if (platform?.factor === "handset") {
    return "Install the abacad app on the phone, then connect it with the device token (or scan its QR when adding the device). It relays commands while running.";
  }
  return "Install the abacad app on the machine, then connect it with the device token. It stays online and relays commands while running.";
}

// Access guideline: agent-pasteable connection instructions. MCP is available on
// every platform; SSH is offered only when the device advertises an ssh_host
// (the real desktop OSes — macOS/Linux/Windows; browsers and phones have none).
function AccessGuide({ device, needsKey }: { device: DeviceView; needsKey: boolean }) {
  const url = `${window.location.protocol}//${window.location.host}/mcp`;
  const mcpCmd = `claude mcp add --transport http abacad ${url} --header "Authorization: Bearer <token>"`;
  return (
    <div className="space-y-5">
      <div>
        <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">MCP</p>
        <p className="mb-2 text-sm leading-6 text-ink-muted">
          Register abacad's MCP endpoint with your agent, then pass this device's{" "}
          <code className="font-mono text-[12px] text-ink">device_id</code> to any tool to target it.
        </p>
        <CopyField value={mcpCmd} />
        <p className="mt-2 text-xs leading-5 text-ink-subtle">
          Generate your token in{" "}
          <Link to="/settings" className="font-medium text-brand hover:underline">
            Settings
          </Link>
          .
        </p>
        <p className="mb-1.5 mt-3 text-xs font-medium text-ink-subtle">Target this device</p>
        <CopyField value={`device_id: ${device.id}`} />
      </div>

      {device.ssh_host && (
        <div className="border-t border-border pt-4">
          <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.16em] text-ink-subtle">SSH</p>
          <p className="mb-2 text-sm leading-6 text-ink-muted">
            Open a shell on this device through the abacad jump host.
          </p>
          <CopyField value={sshCommand(device.ssh_host)} />
          {needsKey && (
            <p className="mt-2 text-xs leading-5 text-ink-subtle">
              Register an{" "}
              <Link to="/settings" className="font-medium text-brand hover:underline">
                SSH key
              </Link>{" "}
              first.
            </p>
          )}
        </div>
      )}
    </div>
  );
}

// Self-contained ssh command for a device: the -J jump host is inlined so it
// works with no ~/.ssh/config entry. The jump host is the ssh_host's parent
// domain (2n6dl6v5icovhlhn.abacad.ai -> abacad.ai).
function sshCommand(sshHost: string): string {
  const jump = sshHost.slice(sshHost.indexOf(".") + 1);
  return `ssh -J ${jump} ${sshHost}`;
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

// Device-scoped sentence for one trail row. The device is implied by the page,
// so — unlike the account-wide Activities page — the name is left off each row.
function activityText(a: ActivityItem, count: number): string {
  const times = count > 1 ? ` ×${count}` : "";
  switch (a.kind) {
    case "device.connected":
      return "Connected";
    case "device.disconnected":
      return `Disconnected${a.detail ? `: ${a.detail}` : ""}`;
    case "device.created":
      return "Device added";
    case "device.renamed":
      return `Renamed${a.detail ? ` to ${a.detail}` : ""}`;
    case "device.deleted":
      return "Device removed";
    case "device.token_rotated":
      return "Device token rotated";
    case "ssh.session":
      return "SSH session opened";
    case "tunnel.opened":
      return `Tunnel opened${a.detail ? ` → ${a.detail}` : ""}`;
    case "command":
      return `${a.method ?? "command"}${times}${a.outcome === "error" && a.detail ? `: ${a.detail}` : ""}`;
    default:
      return `${a.kind}${a.detail ? `: ${a.detail}` : ""}`;
  }
}

function activityIcon(kind: string) {
  switch (kind) {
    case "device.connected":
      return Plug;
    case "device.disconnected":
      return Unplug;
    case "ssh.session":
      return KeyRound;
    case "tunnel.opened":
      return Cable;
    case "command":
      return TerminalSquare;
    default:
      return kind.startsWith("device.") ? Smartphone : TerminalSquare;
  }
}

// Collapse a run of consecutive identical commands (same method/source/outcome)
// into one row so the dashboard's ~3s screenshot polling doesn't bury everything
// else. Non-command rows (SSH, tunnels, connects) always stand alone.
function collapseCommands(items: ActivityItem[]): { first: ActivityItem; count: number }[] {
  const rows: { first: ActivityItem; count: number }[] = [];
  for (const item of items) {
    const prev = rows[rows.length - 1];
    if (
      prev &&
      item.kind === "command" &&
      prev.first.kind === "command" &&
      prev.first.method === item.method &&
      prev.first.source === item.source &&
      prev.first.outcome === item.outcome
    ) {
      prev.count += 1;
    } else {
      rows.push({ first: item, count: 1 });
    }
  }
  return rows;
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

// Recent activity, rendered to sit inside the Activities column card: no outer
// chrome of its own (the card provides it), and a capped scroll height so a long
// trail doesn't stretch the whole column row.
function EventLog({ events }: { events: ActivityItem[] | null }) {
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
    return <p className="py-6 text-center text-sm text-ink-muted">No recent activity for this device yet.</p>;
  }
  return (
    <ul className="max-h-[420px] divide-y divide-border overflow-y-auto">
      {collapseCommands(events).map(({ first: a, count }) => {
        const Icon = activityIcon(a.kind);
        return (
          <li key={a.id} className="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
            <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-border bg-canvas text-ink-muted">
              <Icon size={14} />
            </span>
            <div className="min-w-0 flex-1">
              <p className="break-words text-sm leading-5 text-ink">{activityText(a, count)}</p>
              <p className="mt-1 font-mono text-[11px] text-ink-subtle">
                {clockTime(a.ts)}
                {a.source ? ` · ${a.source}` : ""}
                {a.duration_ms ? ` · ${a.duration_ms}ms` : ""}
              </p>
            </div>
            {a.kind === "command" && (
              <span
                className={`mt-1 shrink-0 rounded px-2 py-1 font-mono text-[10px] font-bold uppercase ${outcomeBadge(a.outcome)}`}
              >
                {a.outcome ?? "pending"}
              </span>
            )}
          </li>
        );
      })}
    </ul>
  );
}
