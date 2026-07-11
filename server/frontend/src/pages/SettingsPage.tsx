import { useEffect, useState } from "react";
import {
  CheckCircle2,
  CircleDashed,
  KeyRound,
  LoaderCircle,
  RefreshCw,
  ShieldCheck,
  TerminalSquare,
} from "lucide-react";
import { api, type McpTokenInfo } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Modal } from "@/components/Modal";
import { CopyField } from "@/components/CopyField";

interface Revealed {
  token: string;
  url: string;
}

function mcpUrl(): string {
  return `${window.location.protocol}//${window.location.host}/mcp`;
}

export function SettingsPage() {
  const [info, setInfo] = useState<McpTokenInfo | null>(null);
  const [revealed, setRevealed] = useState<Revealed | null>(null);
  const [confirmRotate, setConfirmRotate] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = async () => {
    try {
      setInfo(await api.mcpToken());
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  useEffect(() => {
    void reload();
  }, []);

  const rotate = async () => {
    setBusy(true);
    setError(null);
    try {
      const result = await api.rotateMcpToken();
      setRevealed({ token: result.mcp_token, url: mcpUrl() });
      setConfirmRotate(false);
      void reload();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const requestRotate = () => {
    if (info?.exists) {
      setConfirmRotate(true);
    } else {
      void rotate();
    }
  };

  const cmd = (result: Revealed) =>
    `claude mcp add --transport http abacad ${result.url} --header "Authorization: Bearer ${result.token}"`;

  return (
    <div>
      <PageHeader
        eyebrow="Workspace settings"
        title="Agent connection"
        description="Configure the single MCP credential your agents use to reach every device in this workspace."
      />

      {error && (
        <div role="alert" className="mb-5 rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
          {error}
        </div>
      )}

      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(300px,0.65fr)]">
        <Card className="overflow-hidden">
          <div className="flex flex-col gap-4 border-b border-border p-5 sm:flex-row sm:items-start sm:justify-between sm:p-6">
            <div className="flex items-start gap-3">
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md bg-brand-soft text-brand">
                <KeyRound size={19} />
              </span>
              <div>
                <h2 className="font-semibold text-ink">MCP access token</h2>
                <p className="mt-1 max-w-2xl text-sm leading-6 text-ink-muted">
                  Sent as a bearer credential with each MCP request. The plaintext is only displayed immediately after generation.
                </p>
              </div>
            </div>
            {info === null ? (
              <span className="inline-flex h-7 w-24 animate-pulse rounded-md bg-surface-raised" />
            ) : (
              <StatusPill active={info.exists} />
            )}
          </div>

          <div className="p-5 sm:p-6">
            <div className="grid gap-px overflow-hidden rounded-md border border-border bg-border sm:grid-cols-2">
              <Metadata label="Created" value={info?.exists ? fmt(info.created_at) : "Not generated"} loading={info === null} />
              <Metadata label="Last used" value={info?.exists ? (info.last_used ? fmt(info.last_used) : "Never") : "Not available"} loading={info === null} />
            </div>

            <div className="mt-6">
              <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">MCP endpoint</p>
              <CopyField value={mcpUrl()} />
            </div>

            <div className="mt-6 flex flex-col gap-3 border-t border-border pt-5 sm:flex-row sm:items-center sm:justify-between">
              <p className="max-w-xl text-sm leading-6 text-ink-muted">
                {info?.exists
                  ? "Rotating invalidates the current token immediately. Existing agent connections must be updated."
                  : "Generate a token to get a ready-to-run command for your agent."}
              </p>
              <Button variant={info?.exists ? "outline" : "default"} onClick={requestRotate} disabled={busy || info === null}>
                {busy ? <LoaderCircle size={16} className="animate-spin" /> : <RefreshCw size={16} />}
                {info?.exists ? "Rotate token" : "Generate token"}
              </Button>
            </div>
          </div>
        </Card>

        <section className="rounded-lg border border-border bg-sidebar p-5 sm:p-6">
          <div className="flex items-center gap-2">
            <TerminalSquare size={18} className="text-brand" />
            <h2 className="font-semibold text-ink">Connection flow</h2>
          </div>
          <ol className="mt-5 space-y-5">
            <Step number="1" title="Generate a token" detail="The secret appears once in a secure reveal." complete={Boolean(info?.exists)} />
            <Step number="2" title="Register the endpoint" detail="Run the provided command in your agent environment." />
            <Step number="3" title="Start issuing commands" detail="Abacad routes each request to the selected online device." />
          </ol>
          <div className="mt-6 flex items-start gap-2.5 border-t border-border pt-5 text-xs leading-5 text-ink-subtle">
            <ShieldCheck size={16} className="mt-0.5 shrink-0 text-brand" />
            Keep the bearer token out of source control and shared logs.
          </div>
        </section>
      </div>

      <Modal
        open={confirmRotate}
        onClose={() => setConfirmRotate(false)}
        title="Rotate MCP token?"
        description="The current credential will stop working immediately."
      >
        <p className="text-sm leading-6 text-ink-muted">
          Any configured agents will lose access until you replace their bearer token with the newly generated value.
        </p>
        <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
          <Button variant="ghost" onClick={() => setConfirmRotate(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={() => void rotate()} disabled={busy}>
            {busy && <LoaderCircle size={16} className="animate-spin" />}
            Rotate token
          </Button>
        </div>
      </Modal>

      <Modal
        open={revealed !== null}
        onClose={() => setRevealed(null)}
        title="MCP token generated"
        description="This secret is shown once. Store it now before closing."
        className="sm:max-w-2xl"
      >
        {revealed && (
          <div className="flex flex-col gap-5">
            <div>
              <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">Registration command</p>
              <CopyField value={cmd(revealed)} />
            </div>
            <div>
              <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">Bearer token</p>
              <CopyField value={revealed.token} />
            </div>
            <div>
              <p className="mb-2 text-xs font-semibold uppercase text-ink-subtle">Endpoint</p>
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
    </div>
  );
}

function PageHeader({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return (
    <header className="mb-7">
      <p className="text-xs font-semibold uppercase text-brand">{eyebrow}</p>
      <h1 className="mt-2 text-2xl font-semibold text-ink sm:text-[28px]">{title}</h1>
      <p className="mt-2 max-w-2xl text-sm leading-6 text-ink-muted">{description}</p>
    </header>
  );
}

function StatusPill({ active }: { active: boolean }) {
  return (
    <span className={`inline-flex h-7 items-center gap-2 self-start rounded-full border px-2.5 text-xs font-semibold ${active ? "border-success/20 bg-success-soft text-success" : "border-border bg-surface-raised text-ink-muted"}`}>
      {active ? <CheckCircle2 size={14} /> : <CircleDashed size={14} />}
      {active ? "Active" : "Not configured"}
    </span>
  );
}

function Metadata({ label, value, loading }: { label: string; value: string; loading: boolean }) {
  return (
    <div className="bg-canvas px-4 py-3.5">
      <p className="text-xs font-medium text-ink-subtle">{label}</p>
      {loading ? <div className="skeleton mt-2 h-4 w-28 rounded" /> : <p className="mt-1 text-sm font-medium text-ink">{value}</p>}
    </div>
  );
}

function Step({
  number,
  title,
  detail,
  complete = false,
}: {
  number: string;
  title: string;
  detail: string;
  complete?: boolean;
}) {
  return (
    <li className="flex gap-3">
      <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full border text-xs font-bold ${complete ? "border-success/25 bg-success-soft text-success" : "border-border-strong bg-surface text-ink-muted"}`}>
        {complete ? <CheckCircle2 size={15} /> : number}
      </span>
      <div>
        <p className="text-sm font-semibold text-ink">{title}</p>
        <p className="mt-0.5 text-xs leading-5 text-ink-subtle">{detail}</p>
      </div>
    </li>
  );
}

function fmt(iso?: string) {
  if (!iso) return "Not available";
  return new Date(iso).toLocaleString();
}
