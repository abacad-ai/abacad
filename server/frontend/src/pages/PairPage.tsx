import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { CheckCircle2, Globe, LoaderCircle, Monitor, Smartphone, Terminal } from "lucide-react";
import { api, ApiError, type PairInfo } from "@/lib/api";
import { platformInfo } from "@/lib/devices";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageHeader } from "@/components/PageHeader";

// PairPage is the human half of `abacad connect` (RFC 8628 device grant). A CLI
// prints a short code; the signed-in user lands here (typically via the printed
// /pair?code=… link), confirms what's connecting, names it, and approves. The
// CLI's next poll then receives the device token — nothing is copy-pasted.

function PlatformBadge({ platform }: { platform: string }) {
  const { label, factor } = platformInfo(platform);
  const Icon = platform === "browser" ? Globe : factor === "handset" ? Smartphone : Monitor;
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-border-strong bg-surface px-3 py-1 text-sm font-semibold text-ink">
      <Icon size={16} className="text-brand" />
      {label}
    </span>
  );
}

export function PairPage() {
  const [params, setParams] = useSearchParams();
  const codeParam = params.get("code") ?? "";

  const [codeInput, setCodeInput] = useState(codeParam);
  const [pairing, setPairing] = useState<PairInfo | null>(null);
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [approved, setApproved] = useState(false);

  // Look a code up (from the URL or the manual field) to confirm it's pending and
  // learn the platform the CLI reported.
  useEffect(() => {
    if (!codeParam) return;
    let live = true;
    setLoading(true);
    setError(null);
    api
      .pairLookup(codeParam)
      .then((p) => {
        if (!live) return;
        setPairing(p);
        setName(`My ${platformInfo(p.platform).label}`);
      })
      .catch((err) => {
        if (!live) return;
        setPairing(null);
        setError(err instanceof ApiError ? err.message : "Could not look up that code");
      })
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [codeParam]);

  const submitCode = (event: React.FormEvent) => {
    event.preventDefault();
    const trimmed = codeInput.trim();
    if (trimmed) setParams({ code: trimmed }, { replace: true });
  };

  const approve = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!pairing) return;
    setBusy(true);
    setError(null);
    try {
      await api.pairApprove(pairing.user_code, name.trim() || "New device", pairing.platform);
      setApproved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not approve this device");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div>
      <PageHeader title="Connect a device" />

      <div className="mx-auto max-w-lg">
        {approved ? (
          <Card className="p-8 text-center">
            <span className="mx-auto flex h-12 w-12 items-center justify-center rounded-full border border-success/25 bg-success-soft text-success">
              <CheckCircle2 size={24} />
            </span>
            <h2 className="mt-4 font-display text-lg font-bold text-ink">Device approved</h2>
            <p className="mt-2 text-sm leading-6 text-ink-muted">
              Return to your terminal — <span className="font-mono">abacad connect</span> will finish
              connecting automatically.
            </p>
          </Card>
        ) : loading ? (
          <Card className="flex items-center justify-center gap-3 p-10 text-sm text-ink-muted">
            <LoaderCircle size={18} className="animate-spin" />
            Looking up code…
          </Card>
        ) : pairing && pairing.status === "pending" ? (
          <Card className="p-6 sm:p-8">
            <div className="flex items-center gap-3">
              <span className="flex h-10 w-10 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
                <Terminal size={20} />
              </span>
              <div className="min-w-0">
                <p className="font-display text-base font-bold text-ink">A device wants to connect</p>
                <p className="text-sm text-ink-muted">
                  Approving adds it to your account and issues its credential.
                </p>
              </div>
            </div>

            <form onSubmit={approve} className="mt-6 flex flex-col gap-5">
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-medium text-ink-muted">Reported platform</span>
                <PlatformBadge platform={pairing.platform} />
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-medium text-ink-muted">Code</span>
                <span className="font-mono text-sm tracking-[0.2em] text-ink">{pairing.user_code}</span>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="pair-name">Device name</Label>
                <Input
                  id="pair-name"
                  autoFocus
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="New device"
                />
              </div>
              {error && (
                <p role="alert" className="text-sm text-danger">
                  {error}
                </p>
              )}
              <Button type="submit" disabled={busy} className="w-full">
                {busy && <LoaderCircle size={16} className="animate-spin" />}
                Approve &amp; connect
              </Button>
            </form>
          </Card>
        ) : (
          // No code yet, or the looked-up code was invalid/expired: let the user
          // (re)enter the code the CLI printed.
          <Card className="p-6 sm:p-8">
            <h2 className="font-display text-base font-bold text-ink">Enter your pairing code</h2>
            <p className="mt-1 text-sm leading-6 text-ink-muted">
              Run <span className="font-mono">abacad connect</span> on your device and type the code it
              shows below.
            </p>
            <form onSubmit={submitCode} className="mt-5 flex flex-col gap-4">
              <Input
                autoFocus
                value={codeInput}
                onChange={(event) => setCodeInput(event.target.value)}
                placeholder="WXYZ-2K7M"
                className="text-center font-mono text-lg uppercase tracking-[0.3em]"
              />
              {error && (
                <p role="alert" className="text-sm text-danger">
                  {error}
                </p>
              )}
              <Button type="submit" disabled={!codeInput.trim()} className="w-full">
                Continue
              </Button>
            </form>
          </Card>
        )}
      </div>
    </div>
  );
}
