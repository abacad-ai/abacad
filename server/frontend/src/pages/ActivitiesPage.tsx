import { useEffect, useMemo, useRef, useState } from "react";
import {
  Activity,
  Cable,
  KeyRound,
  ListFilter,
  LoaderCircle,
  LogIn,
  Plug,
  RefreshCw,
  Smartphone,
  TerminalSquare,
} from "lucide-react";
import { api, type ActivityItem, type DeviceView } from "@/lib/api";
import { clockTime } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";

const PAGE_SIZE = 50;

// Filter categories map to the backend's kind-prefix filter ("" = everything).
const CATEGORIES = [
  { value: "", label: "All" },
  { value: "command", label: "Commands" },
  { value: "device", label: "Devices" },
  { value: "auth", label: "Sign-ins" },
  { value: "ssh", label: "SSH" },
  { value: "tunnel", label: "Tunnels" },
  { value: "mcp", label: "MCP" },
] as const;

const SOURCES = [
  { value: "", label: "Any source" },
  { value: "agent", label: "Agent" },
  { value: "dashboard", label: "Dashboard" },
  { value: "ssh", label: "SSH" },
  { value: "tunnel", label: "Tunnel" },
] as const;

// A row is either one activity or a run of consecutive identical commands
// (same device/method/source/outcome) collapsed so agent bursts stay scannable.
interface Row {
  first: ActivityItem; // newest in the run
  last: ActivityItem; // oldest in the run
  count: number;
}

function collapse(items: ActivityItem[]): Row[] {
  const rows: Row[] = [];
  for (const item of items) {
    const prev = rows[rows.length - 1];
    if (
      prev &&
      item.kind === "command" &&
      prev.first.kind === "command" &&
      prev.first.device_id === item.device_id &&
      prev.first.method === item.method &&
      prev.first.source === item.source &&
      prev.first.outcome === item.outcome
    ) {
      prev.last = item;
      prev.count += 1;
    } else {
      rows.push({ first: item, last: item, count: 1 });
    }
  }
  return rows;
}

function dayLabel(ts: number): string {
  const date = new Date(ts);
  const today = new Date();
  const yesterday = new Date(today);
  yesterday.setDate(today.getDate() - 1);
  const sameDay = (a: Date, b: Date) =>
    a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
  if (sameDay(date, today)) return "Today";
  if (sameDay(date, yesterday)) return "Yesterday";
  return date.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" });
}

function outcomeBadge(outcome?: string): string {
  switch (outcome) {
    case "ok":
      return "bg-success-soft text-success";
    case "failed":
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

function categoryOf(kind: string): string {
  const dot = kind.indexOf(".");
  return dot === -1 ? kind : kind.slice(0, dot);
}

function rowIcon(kind: string) {
  switch (categoryOf(kind)) {
    case "auth":
      return LogIn;
    case "device":
      return Smartphone;
    case "command":
      return TerminalSquare;
    case "ssh":
      return KeyRound;
    case "tunnel":
      return Cable;
    case "mcp":
      return Plug;
    default:
      return Activity;
  }
}

// rowText renders the human sentence for a row; device names resolve via the
// account's device list (deleted devices fall back to their id).
function rowText(row: Row, deviceName: (id?: string) => string): string {
  const a = row.first;
  const dev = deviceName(a.device_id);
  switch (a.kind) {
    case "auth.login":
      return "Signed in";
    case "auth.login_failed":
      return `Failed sign-in attempt${a.detail ? ` (${a.detail})` : ""}`;
    case "auth.logout":
      return "Signed out";
    case "auth.register":
      return `Account created${a.detail ? ` (${a.detail})` : ""}`;
    case "device.created":
      return `Device added: ${a.detail || dev}`;
    case "device.renamed":
      return `Device renamed to ${a.detail || dev}`;
    case "device.deleted":
      return `Device removed: ${a.detail || dev}`;
    case "device.token_rotated":
      return `Device token rotated for ${dev}`;
    case "device.connected":
      return `${dev} connected`;
    case "device.disconnected":
      return `${dev} disconnected${a.detail ? `: ${a.detail}` : ""}`;
    case "mcp.token_rotated":
      return "MCP token rotated";
    case "ssh.key_added":
      return `SSH key added${a.detail ? `: ${a.detail}` : ""}`;
    case "ssh.key_removed":
      return `SSH key removed${a.detail ? `: ${a.detail}` : ""}`;
    case "ssh.session":
      return `SSH session opened to ${dev}`;
    case "tunnel.opened":
      return `Tunnel opened via ${dev}${a.detail ? ` → ${a.detail}` : ""}`;
    case "command": {
      const count = row.count > 1 ? ` ×${row.count}` : "";
      const err = a.outcome === "error" && a.detail ? `: ${a.detail}` : "";
      return `${a.method}${count} on ${dev}${err}`;
    }
    default:
      return `${a.kind}${a.detail ? `: ${a.detail}` : ""}`;
  }
}

export function ActivitiesPage() {
  const [items, setItems] = useState<ActivityItem[] | null>(null);
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [nextBefore, setNextBefore] = useState<number | undefined>();
  const [category, setCategory] = useState<string>("");
  const [deviceId, setDeviceId] = useState<string>("");
  const [source, setSource] = useState<string>("");
  const [error, setError] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const generation = useRef(0);

  useEffect(() => {
    api.devices().then(setDevices, () => {});
  }, []);

  useEffect(() => {
    const gen = ++generation.current;
    setItems(null);
    setError(null);
    api
      .activities({ kind: category, device: deviceId, source, limit: PAGE_SIZE })
      .then((result) => {
        if (generation.current !== gen) return;
        setItems(result.activities);
        setNextBefore(result.next_before);
      })
      .catch((err) => {
        if (generation.current === gen) setError((err as Error).message);
      });
  }, [category, deviceId, source]);

  const loadOlder = async () => {
    if (!nextBefore || loadingMore) return;
    const gen = generation.current;
    setLoadingMore(true);
    try {
      const result = await api.activities({
        kind: category,
        device: deviceId,
        source,
        before: nextBefore,
        limit: PAGE_SIZE,
      });
      if (generation.current !== gen) return;
      setItems((prev) => [...(prev ?? []), ...result.activities]);
      setNextBefore(result.next_before);
    } catch (err) {
      if (generation.current === gen) setError((err as Error).message);
    } finally {
      setLoadingMore(false);
    }
  };

  const deviceName = (id?: string) => {
    if (!id) return "device";
    return devices.find((d) => d.id === id)?.name ?? id;
  };

  // Collapse command bursts, then group rows by day for the timeline headers.
  const dayGroups = useMemo(() => {
    if (!items) return [];
    const rows = collapse(items);
    const groups: { label: string; rows: Row[] }[] = [];
    for (const row of rows) {
      const label = dayLabel(row.first.ts);
      const group = groups[groups.length - 1];
      if (group && group.label === label) group.rows.push(row);
      else groups.push({ label, rows: [row] });
    }
    return groups;
  }, [items, devices]);

  const select =
    "h-10 rounded-md border border-border bg-surface px-3 text-[13px] font-medium text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand";

  return (
    <div>
      <header className="mb-7">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[0.22em] text-brand">
          console / activities
        </p>
        <h1 className="mt-3 font-display text-3xl font-bold leading-tight text-ink sm:text-4xl">Activities</h1>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">
          The workspace trail: sign-ins, credential changes, device connections, and every command an agent ran.
        </p>
      </header>

      <div className="mb-6 flex flex-wrap items-center gap-2.5">
        <span className="flex h-10 items-center gap-1.5 font-mono text-[11px] uppercase tracking-wider text-ink-subtle">
          <ListFilter size={14} />
          Filter
        </span>
        <div role="group" aria-label="Filter by category" className="flex flex-wrap items-center gap-1 rounded-full border border-border bg-surface p-1">
          {CATEGORIES.map((c) => (
            <button
              key={c.value}
              type="button"
              onClick={() => setCategory(c.value)}
              aria-pressed={category === c.value}
              className={`flex h-8 items-center rounded-full px-3 text-[13px] font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
                category === c.value ? "bg-brand-soft text-brand" : "text-ink-muted hover:text-ink"
              }`}
            >
              {c.label}
            </button>
          ))}
        </div>
        <select aria-label="Filter by device" className={select} value={deviceId} onChange={(e) => setDeviceId(e.target.value)}>
          <option value="">All devices</option>
          {devices.map((d) => (
            <option key={d.id} value={d.id}>
              {d.name}
            </option>
          ))}
        </select>
        <select aria-label="Filter by source" className={select} value={source} onChange={(e) => setSource(e.target.value)}>
          {SOURCES.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
      </div>

      {error && (
        <Card className="border-danger/25 p-6 text-center">
          <p className="text-sm font-semibold text-danger">Unable to load activities</p>
          <p className="mt-1 text-sm text-ink-muted">{error}</p>
          <Button variant="outline" className="mt-5" onClick={() => setCategory((c) => c)}>
            <RefreshCw size={16} />
            Try again
          </Button>
        </Card>
      )}

      {!error && items === null && (
        <div className="space-y-2" aria-label="Loading activities">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <div key={i} className="skeleton h-14 rounded-md" />
          ))}
        </div>
      )}

      {!error && items !== null && items.length === 0 && (
        <section className="rounded-[10px] border border-dashed border-border-strong bg-surface px-5 py-14 text-center sm:py-20">
          <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
            <Activity size={23} />
          </span>
          <h2 className="mt-4 font-display text-lg font-bold text-ink">No activity yet</h2>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink-muted">
            Sign-ins, device connections, and agent commands will appear here as they happen.
          </p>
        </section>
      )}

      {!error && items !== null && items.length > 0 && (
        <div className="space-y-7">
          {dayGroups.map((group) => (
            <section key={group.label} aria-label={group.label}>
              <h2 className="mb-2.5 font-mono text-[11px] font-medium uppercase tracking-[0.2em] text-ink-subtle">
                {group.label}
              </h2>
              <ul className="divide-y divide-border overflow-hidden rounded-[10px] border border-border bg-surface">
                {group.rows.map((row) => {
                  const Icon = rowIcon(row.first.kind);
                  const isCommand = row.first.kind === "command";
                  const dashboardScreenshot =
                    isCommand && row.first.method === "screenshot" && row.first.source === "dashboard";
                  return (
                    <li
                      key={row.first.id}
                      className={`flex items-start gap-3 px-3.5 py-3 ${dashboardScreenshot ? "opacity-60" : ""}`}
                    >
                      <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-border bg-canvas text-ink-muted">
                        <Icon size={14} />
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="break-words text-sm leading-5 text-ink">{rowText(row, deviceName)}</p>
                        <p className="mt-1 font-mono text-[11px] text-ink-subtle">
                          {row.count > 1
                            ? `${clockTime(row.last.ts)} – ${clockTime(row.first.ts)}`
                            : clockTime(row.first.ts)}
                          {row.first.source ? ` · ${row.first.source}` : ""}
                          {row.count === 1 && row.first.duration_ms ? ` · ${row.first.duration_ms}ms` : ""}
                        </p>
                      </div>
                      {isCommand && (
                        <span
                          className={`mt-1 shrink-0 rounded px-2 py-1 font-mono text-[10px] font-bold uppercase ${outcomeBadge(row.first.outcome)}`}
                        >
                          {row.first.outcome ?? "pending"}
                        </span>
                      )}
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}

          {nextBefore !== undefined && (
            <div className="flex justify-center">
              <Button variant="outline" onClick={() => void loadOlder()} disabled={loadingMore}>
                {loadingMore && <LoaderCircle size={16} className="animate-spin" />}
                Load older
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
