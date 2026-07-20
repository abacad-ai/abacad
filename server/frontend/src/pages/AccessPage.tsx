import { useEffect, useState } from "react";
import { CheckCircle2, KeyRound, LoaderCircle, Pencil, Plus, Trash2 } from "lucide-react";
import { api, type ApiKey, type DeviceView, type KeyInput, type NewApiKey } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";
import { PageHeader } from "@/components/PageHeader";
import { cn } from "@/lib/utils";

// The device methods a key can be scoped to, grouped for the picker. Kept in sync
// with the server's protocol.Methods (the server rejects anything unknown).
const METHOD_GROUPS: { label: string; methods: [string, string][] }[] = [
  { label: "General", methods: [["screenshot", "screenshot"], ["input_text", "input_text"]] },
  {
    label: "Mobile",
    methods: [
      ["tap", "tap"],
      ["long_press", "long_press"],
      ["swipe", "swipe"],
      ["back", "back"],
      ["home", "home"],
      ["recents", "recents"],
    ],
  },
  {
    label: "Desktop",
    methods: [
      ["click", "click"],
      ["right_click", "right_click"],
      ["drag", "drag"],
      ["scroll", "scroll"],
      ["press_keys", "press_keys"],
      ["composite", "composite"],
    ],
  },
  { label: "Browser", methods: [["execute", "execute (run JS)"]] },
];

export function AccessPage() {
  const [keys, setKeys] = useState<ApiKey[] | null>(null);
  const [devices, setDevices] = useState<DeviceView[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<ApiKey | "new" | null>(null);
  const [revealed, setRevealed] = useState<{ secret: string; url: string } | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<ApiKey | null>(null);
  const [busy, setBusy] = useState(false);

  const reload = async () => {
    try {
      const [ks, ds] = await Promise.all([api.keys(), api.devices()]);
      setKeys(ks);
      setDevices(ds);
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  useEffect(() => {
    void reload();
  }, []);

  const onSaved = (created: NewApiKey | null) => {
    setEditing(null);
    if (created) setRevealed({ secret: created.secret, url: created.mcp_url });
    void reload();
  };

  const remove = async (key: ApiKey) => {
    setBusy(true);
    setError(null);
    try {
      await api.deleteKey(key.id);
      setConfirmDelete(null);
      await reload();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <PageHeader
        title="Access"
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus size={16} />
            New key
          </Button>
        }
      />

      {error && (
        <div role="alert" className="mb-5 rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
          {error}
        </div>
      )}

      <Card className="overflow-hidden">
        <div className="flex items-start gap-3 border-b border-border p-5 sm:p-6">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
            <KeyRound size={19} />
          </span>
          <div>
            <h2 className="font-display text-lg font-bold text-ink">API keys</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-ink-muted">
              Each key is a bearer credential for the MCP endpoint and the tunnel, scoped to the
              devices and methods you allow. The secret is shown once, when you create the key.
            </p>
          </div>
        </div>

        <div className="p-5 sm:p-6">
          {keys === null ? (
            <div className="skeleton h-16 w-full rounded-md" />
          ) : keys.length === 0 ? (
            <p className="rounded-md border border-dashed border-border bg-canvas px-4 py-6 text-center text-sm text-ink-subtle">
              No keys yet. Create one to connect an agent.
            </p>
          ) : (
            <ul className="grid gap-px overflow-hidden rounded-md border border-border bg-border">
              {keys.map((k) => (
                <li key={k.id} className="flex items-center gap-3 bg-canvas px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-ink">{k.name || "Unnamed key"}</p>
                    <p className="mt-0.5 truncate text-xs text-ink-muted">{scopeSummary(k, devices)}</p>
                    <p className="mt-0.5 text-xs text-ink-subtle">
                      Added {fmt(k.created_at)}
                      {k.last_used ? ` · last used ${fmt(k.last_used)}` : " · never used"}
                    </p>
                  </div>
                  <button
                    type="button"
                    onClick={() => setEditing(k)}
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-surface-hover hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    title="Edit key"
                    aria-label={`Edit key ${k.name}`}
                  >
                    <Pencil size={16} />
                  </button>
                  <button
                    type="button"
                    onClick={() => setConfirmDelete(k)}
                    className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-danger-soft hover:text-danger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                    title="Delete key"
                    aria-label={`Delete key ${k.name}`}
                  >
                    <Trash2 size={17} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </Card>

      {editing && (
        <KeyFormModal
          initial={editing === "new" ? null : editing}
          devices={devices}
          onClose={() => setEditing(null)}
          onSaved={onSaved}
        />
      )}

      <Modal
        open={revealed !== null}
        onClose={() => setRevealed(null)}
        title="API key created"
        description="This secret is shown once. Store it now before closing."
        className="sm:max-w-2xl"
      >
        {revealed && (
          <div className="flex flex-col gap-5">
            <div>
              <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Registration command</p>
              <CopyField
                value={`claude mcp add --transport http abacad ${revealed.url} --header "Authorization: Bearer ${revealed.secret}"`}
              />
            </div>
            <div>
              <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Bearer token</p>
              <CopyField value={revealed.secret} />
            </div>
            <div>
              <p className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">Endpoint</p>
              <CopyField value={revealed.url} />
            </div>
            <div className="flex justify-end border-t border-border pt-5">
              <Button onClick={() => setRevealed(null)}>
                <CheckCircle2 size={17} />
                I stored the token
              </Button>
            </div>
          </div>
        )}
      </Modal>

      <Modal
        open={confirmDelete !== null}
        onClose={() => setConfirmDelete(null)}
        title="Delete API key?"
        description="Any agent using this key loses access immediately."
      >
        {confirmDelete && (
          <>
            <p className="text-sm text-ink-muted">
              <span className="font-medium text-ink">{confirmDelete.name || "Unnamed key"}</span> will stop working at once.
            </p>
            <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button variant="ghost" onClick={() => setConfirmDelete(null)}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={() => void remove(confirmDelete)} disabled={busy}>
                {busy && <LoaderCircle size={16} className="animate-spin" />}
                Delete key
              </Button>
            </div>
          </>
        )}
      </Modal>
    </div>
  );
}

// KeyFormModal creates a new key (initial=null) or edits an existing one. The two
// "All" radios persist as wildcards that also cover future devices/methods.
function KeyFormModal({
  initial,
  devices,
  onClose,
  onSaved,
}: {
  initial: ApiKey | null;
  devices: DeviceView[];
  onClose: () => void;
  onSaved: (created: NewApiKey | null) => void;
}) {
  const [name, setName] = useState(initial?.name ?? "");
  const [allDevices, setAllDevices] = useState(initial?.all_devices ?? true);
  const [deviceIds, setDeviceIds] = useState<string[]>(initial?.device_ids ?? []);
  const [allMethods, setAllMethods] = useState(initial?.all_methods ?? true);
  const [methods, setMethods] = useState<string[]>(initial?.methods ?? []);
  const [allowTunnel, setAllowTunnel] = useState(initial?.allow_tunnel ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const toggle = (list: string[], set: (v: string[]) => void, value: string) =>
    set(list.includes(value) ? list.filter((v) => v !== value) : [...list, value]);

  const submit = async () => {
    setBusy(true);
    setError(null);
    const body: KeyInput = {
      name: name.trim(),
      all_devices: allDevices,
      device_ids: allDevices ? [] : deviceIds,
      all_methods: allMethods,
      methods: allMethods ? [] : methods,
      allow_tunnel: allowTunnel,
    };
    try {
      if (initial) {
        await api.updateKey(initial.id, body);
        onSaved(null);
      } else {
        const created = await api.createKey(body);
        onSaved(created);
      }
    } catch (err) {
      setError((err as Error).message);
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={initial ? "Edit API key" : "New API key"}
      description="Restrict what this key can reach. Choose “All” to also cover devices or methods you add later."
      className="sm:max-w-2xl"
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        {error && (
          <div role="alert" className="rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
            {error}
          </div>
        )}

        <div>
          <Label htmlFor="key-name" className="mb-1.5 block">
            Name
          </Label>
          <Input id="key-name" placeholder="e.g. laptop agent" value={name} onChange={(e) => setName(e.target.value)} maxLength={80} />
        </div>

        {/* Devices */}
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-muted">Devices</legend>
          <Choice checked={allDevices} onChange={() => setAllDevices(true)} label="All devices" hint="Includes devices you add in the future." />
          <Choice checked={!allDevices} onChange={() => setAllDevices(false)} label="Specific devices" />
          {!allDevices && (
            <div className="ml-6 grid gap-px overflow-hidden rounded-md border border-border bg-border">
              {devices.length === 0 ? (
                <p className="bg-canvas px-4 py-3 text-sm text-ink-subtle">You have no devices yet.</p>
              ) : (
                devices.map((d) => (
                  <CheckRow
                    key={d.id}
                    checked={deviceIds.includes(d.id)}
                    onChange={() => toggle(deviceIds, setDeviceIds, d.id)}
                    label={d.name || "Unnamed device"}
                    sub={d.platform}
                  />
                ))
              )}
            </div>
          )}
        </fieldset>

        {/* Methods */}
        <fieldset className="flex flex-col gap-3">
          <legend className="mb-1 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-muted">Methods</legend>
          <Choice checked={allMethods} onChange={() => setAllMethods(true)} label="All methods" hint="Includes methods added in future releases." />
          <Choice checked={!allMethods} onChange={() => setAllMethods(false)} label="Specific methods" />
          {!allMethods && (
            <div className="ml-6 flex flex-col gap-4">
              {METHOD_GROUPS.map((group) => (
                <div key={group.label}>
                  <p className="mb-2 text-xs font-medium text-ink-subtle">{group.label}</p>
                  <div className="grid gap-px overflow-hidden rounded-md border border-border bg-border sm:grid-cols-2">
                    {group.methods.map(([value, label]) => (
                      <CheckRow
                        key={value}
                        checked={methods.includes(value)}
                        onChange={() => toggle(methods, setMethods, value)}
                        label={label}
                        mono
                      />
                    ))}
                  </div>
                </div>
              ))}
            </div>
          )}
        </fieldset>

        {/* Tunnel */}
        <fieldset>
          <legend className="mb-2 font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-muted">Tunnel</legend>
          <CheckRow
            checked={allowTunnel}
            onChange={() => setAllowTunnel(!allowTunnel)}
            label="Allow raw TCP tunnels"
            sub="ssh, scp, databases, and other host:port targets reachable from the device."
            bordered
          />
        </fieldset>

        <div className="flex flex-col-reverse gap-2 border-t border-border pt-5 sm:flex-row sm:justify-end">
          <Button type="button" variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" disabled={busy}>
            {busy && <LoaderCircle size={16} className="animate-spin" />}
            {initial ? "Save changes" : "Create key"}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

// Choice is a labeled radio option (used for the All / Specific toggles).
function Choice({
  checked,
  onChange,
  label,
  hint,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
  hint?: string;
}) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <input type="radio" checked={checked} onChange={onChange} className="mt-0.5 h-4 w-4 accent-brand" />
      <span className="min-w-0">
        <span className="text-sm font-medium text-ink">{label}</span>
        {hint && <span className="mt-0.5 block text-xs text-ink-subtle">{hint}</span>}
      </span>
    </label>
  );
}

// CheckRow is a labeled checkbox row for the device / method / tunnel lists.
function CheckRow({
  checked,
  onChange,
  label,
  sub,
  mono,
  bordered,
}: {
  checked: boolean;
  onChange: () => void;
  label: string;
  sub?: string;
  mono?: boolean;
  bordered?: boolean;
}) {
  return (
    <label
      className={cn(
        "flex cursor-pointer items-center gap-3 bg-canvas px-4 py-2.5",
        bordered && "rounded-md border border-border",
      )}
    >
      <input type="checkbox" checked={checked} onChange={onChange} className="h-4 w-4 shrink-0 accent-brand" />
      <span className="min-w-0">
        <span className={cn("text-sm text-ink", mono && "font-mono text-xs")}>{label}</span>
        {sub && <span className="mt-0.5 block text-xs text-ink-subtle">{sub}</span>}
      </span>
    </label>
  );
}

function scopeSummary(k: ApiKey, devices: DeviceView[]): string {
  const parts: string[] = [];
  if (k.all_devices) {
    parts.push("All devices");
  } else if (k.device_ids.length === 1) {
    const d = devices.find((x) => x.id === k.device_ids[0]);
    parts.push(d ? d.name || "1 device" : "1 device");
  } else {
    parts.push(`${k.device_ids.length} devices`);
  }
  parts.push(k.all_methods ? "All methods" : `${k.methods.length} method${k.methods.length === 1 ? "" : "s"}`);
  if (k.allow_tunnel) parts.push("Tunnel");
  return parts.join(" · ");
}

function fmt(iso?: string) {
  if (!iso) return "unknown";
  return new Date(iso).toLocaleDateString();
}
