import { useEffect, useState } from "react";
import { KeyRound, LoaderCircle, Plus, Trash2 } from "lucide-react";
import { api, type SshKey } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Modal } from "@/components/Modal";

// SshKeysCard lets an account register the SSH public keys that authorize the
// jump host (ssh <device>.<base-domain>). Keys are matched by fingerprint; the
// public key is not a secret, so no reveal-once flow is needed.
export function SshKeysCard() {
  const [keys, setKeys] = useState<SshKey[] | null>(null);
  const [name, setName] = useState("");
  const [pubkey, setPubkey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<SshKey | null>(null);

  const reload = async () => {
    try {
      setKeys(await api.sshKeys());
      setError(null);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  useEffect(() => {
    void reload();
  }, []);

  const add = async () => {
    if (!pubkey.trim()) return;
    setBusy(true);
    setError(null);
    try {
      await api.addSshKey(name.trim(), pubkey.trim());
      setName("");
      setPubkey("");
      await reload();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const remove = async (key: SshKey) => {
    setBusy(true);
    setError(null);
    try {
      await api.deleteSshKey(key.id);
      setConfirmDelete(null);
      await reload();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="mt-5 overflow-hidden">
      <div className="flex flex-col gap-4 border-b border-border p-5 sm:flex-row sm:items-start sm:justify-between sm:p-6">
        <div className="flex items-start gap-3">
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
            <KeyRound size={19} />
          </span>
          <div>
            <h2 className="font-display text-lg font-bold text-ink">SSH access keys</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-ink-muted">
              Authorize the public keys that may reach your devices over SSH. A key identifies
              your account at the jump host; you then <code className="font-mono text-xs text-brand">ssh</code> to any
              of your devices. Public keys are safe to share — paste the contents of your{" "}
              <code className="font-mono text-xs text-brand">.pub</code> file.
            </p>
          </div>
        </div>
      </div>

      <div className="p-5 sm:p-6">
        {error && (
          <div role="alert" className="mb-5 rounded-md border border-danger/25 bg-danger-soft px-4 py-3 text-sm text-danger">
            {error}
          </div>
        )}

        {/* Existing keys */}
        {keys === null ? (
          <div className="skeleton h-16 w-full rounded-md" />
        ) : keys.length === 0 ? (
          <p className="rounded-md border border-dashed border-border bg-canvas px-4 py-6 text-center text-sm text-ink-subtle">
            No SSH keys yet. Add one below to enable <code className="font-mono text-xs text-brand">ssh</code> access.
          </p>
        ) : (
          <ul className="grid gap-px overflow-hidden rounded-md border border-border bg-border">
            {keys.map((k) => (
              <li key={k.id} className="flex items-center gap-3 bg-canvas px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium text-ink">{k.name || "Unnamed key"}</p>
                  <p className="truncate font-mono text-xs text-ink-subtle">{k.fingerprint}</p>
                  <p className="mt-0.5 text-xs text-ink-subtle">
                    Added {fmt(k.created_at)}
                    {k.last_used ? ` · last used ${fmt(k.last_used)}` : " · never used"}
                  </p>
                </div>
                <button
                  type="button"
                  onClick={() => setConfirmDelete(k)}
                  className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-ink-muted transition-colors hover:bg-danger-soft hover:text-danger focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                  title="Remove key"
                  aria-label={`Remove key ${k.name || k.fingerprint}`}
                >
                  <Trash2 size={17} />
                </button>
              </li>
            ))}
          </ul>
        )}

        {/* Add key form */}
        <form
          className="mt-6 flex flex-col gap-3 border-t border-border pt-5"
          onSubmit={(e) => {
            e.preventDefault();
            void add();
          }}
        >
          <div>
            <label htmlFor="ssh-key-name" className="mb-1.5 block font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">
              Label <span className="font-normal normal-case tracking-normal">(optional)</span>
            </label>
            <Input
              id="ssh-key-name"
              placeholder="e.g. work laptop"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={80}
            />
          </div>
          <div>
            <label htmlFor="ssh-key-value" className="mb-1.5 block font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-subtle">
              Public key
            </label>
            <textarea
              id="ssh-key-value"
              placeholder="ssh-ed25519 AAAAC3Nza... you@laptop"
              value={pubkey}
              onChange={(e) => setPubkey(e.target.value)}
              rows={3}
              spellCheck={false}
              className="flex w-full resize-y rounded-md border border-border-strong bg-canvas px-3.5 py-2.5 font-mono text-xs text-ink placeholder:text-ink-subtle focus-visible:border-brand focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/25"
            />
          </div>
          <div className="flex justify-end">
            <Button type="submit" disabled={busy || !pubkey.trim()}>
              {busy ? <LoaderCircle size={16} className="animate-spin" /> : <Plus size={16} />}
              Add key
            </Button>
          </div>
        </form>

        <p className="mt-6 border-t border-border pt-5 text-sm leading-6 text-ink-muted">
          Once a key is added, connect from any device's page — each carries a ready-to-run
          <code className="mx-1 font-mono text-xs text-brand">ssh</code>command.
        </p>
      </div>

      <Modal
        open={confirmDelete !== null}
        onClose={() => setConfirmDelete(null)}
        title="Remove SSH key?"
        description="This key will no longer be able to reach your devices over SSH."
      >
        {confirmDelete && (
          <>
            <p className="font-mono text-xs text-ink-muted">{confirmDelete.fingerprint}</p>
            <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button variant="ghost" onClick={() => setConfirmDelete(null)}>
                Cancel
              </Button>
              <Button variant="destructive" onClick={() => void remove(confirmDelete)} disabled={busy}>
                {busy && <LoaderCircle size={16} className="animate-spin" />}
                Remove key
              </Button>
            </div>
          </>
        )}
      </Modal>
    </Card>
  );
}

function fmt(iso?: string) {
  if (!iso) return "unknown";
  return new Date(iso).toLocaleDateString();
}
