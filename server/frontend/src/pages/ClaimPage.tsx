import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { CheckCircle2, Globe, LoaderCircle, Monitor, Smartphone } from "lucide-react";
import { api, ApiError, type ClaimPreview } from "@/lib/api";
import { claimPath, normalizeClaimCode, normalizeDeviceId, savePendingClaim } from "@/lib/claim";
import { useAuth } from "@/auth";
import { platformInfo } from "@/lib/devices";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { PageHeader } from "@/components/PageHeader";

// ClaimPage is the human half of device-first self-enrollment. A freshly
// installed client shows its device id and a short-lived claim code on its own
// screen; the person holding that device types both here to add it to their
// account.
//
// Public on purpose: holding the id AND the code is the proof, and both are
// readable only off the device itself. The flow is preview-first — confirm which
// machine you're claiming, THEN sign in — because being asked to create an
// account before seeing anything work is exactly the friction this replaces.

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

export function ClaimPage() {
  const { me, loading: authLoading } = useAuth();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();

  const idParam = params.get("d") ?? "";
  const codeParam = params.get("c") ?? "";

  const [idInput, setIdInput] = useState(idParam);
  const [codeInput, setCodeInput] = useState(codeParam);
  const [device, setDevice] = useState<ClaimPreview | null>(null);
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [accepted, setAccepted] = useState(false);

  const normalizedId = normalizeDeviceId(idParam);
  const normalizedCode = normalizeClaimCode(codeParam);

  // Preview whatever is in the URL. Both values are required — the id alone
  // reveals nothing, which is what keeps this from being a device-id oracle.
  useEffect(() => {
    if (!normalizedId || !normalizedCode) return;
    let live = true;
    setLoading(true);
    setError(null);
    api
      .claimPreview(normalizedId, normalizedCode)
      .then((d) => {
        if (!live) return;
        setDevice(d);
        setName(d.name && d.name !== "New device" ? d.name : `My ${platformInfo(d.platform).label}`);
      })
      .catch((err) => {
        if (!live) return;
        setDevice(null);
        // The server returns one identical 404 for unknown id / wrong code /
        // expired / already claimed, so the message stays deliberately vague.
        setError(
          err instanceof ApiError && err.status !== 404
            ? err.message
            : "No device matches that ID and code. Check both, and note the code changes every few minutes.",
        );
      })
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [normalizedId, normalizedCode]);

  const submitLookup = (event: React.FormEvent) => {
    event.preventDefault();
    const id = normalizeDeviceId(idInput);
    const code = normalizeClaimCode(codeInput);
    if (!id || !code) {
      setError("Enter the 16-letter device ID and the 8-character claim code shown on the device.");
      return;
    }
    setParams({ d: id, c: code }, { replace: true });
  };

  const claim = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!device || !accepted) return;

    // Not signed in yet: park the claim and come back after auth. Both halves
    // matter — sessionStorage survives the Google round trip, and ?next= tells
    // the server where to send us if storage is unavailable.
    if (!me) {
      const pending = { deviceId: device.device_id, claimCode: normalizedCode };
      savePendingClaim(pending);
      navigate(`/login?next=${encodeURIComponent(claimPath(pending))}`);
      return;
    }

    setBusy(true);
    setError(null);
    try {
      const res = await api.claimDevice(device.device_id, normalizedCode, name.trim(), accepted);
      navigate(`/devices/${res.device_id}`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not add this device");
      setBusy(false);
    }
  };

  return (
    <div>
      <PageHeader title="Add a device" />

      <div className="mx-auto max-w-lg">
        {loading ? (
          <Card className="flex items-center justify-center gap-3 p-10 text-sm text-ink-muted">
            <LoaderCircle size={18} className="animate-spin" />
            Looking for that device…
          </Card>
        ) : device ? (
          <Card className="p-6 sm:p-8">
            <div className="flex items-center gap-3">
              <span className="flex h-10 w-10 items-center justify-center rounded-md border border-success/25 bg-success-soft text-success">
                <CheckCircle2 size={20} />
              </span>
              <div className="min-w-0">
                <p className="font-display text-base font-bold text-ink">Found your device</p>
                <p className="text-sm text-ink-muted">
                  {device.online ? "It's online and waiting to be added." : "It isn't reporting in right now."}
                </p>
              </div>
            </div>

            <form onSubmit={claim} className="mt-6 flex flex-col gap-5">
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-medium text-ink-muted">Platform</span>
                <PlatformBadge platform={device.platform} />
              </div>
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-medium text-ink-muted">Device ID</span>
                <span className="font-mono text-sm text-ink">{device.device_id}</span>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="claim-name">Device name</Label>
                <Input
                  id="claim-name"
                  autoFocus
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="New device"
                />
              </div>
              <div className="rounded-md border border-border bg-surface-2 p-3 text-sm leading-6 text-ink-muted">
                Adding this device lets an agent you direct <strong>see its screen, read its
                on-screen text, inject taps and keystrokes, transfer files, and record the
                screen</strong>. Only add a device you own or are authorized to operate, and make
                sure anyone who uses it knows it can be controlled remotely.
              </div>
              <label className="flex cursor-pointer items-start gap-3 text-sm leading-6">
                <input
                  type="checkbox"
                  checked={accepted}
                  onChange={(event) => setAccepted(event.target.checked)}
                  className="mt-1 h-4 w-4 shrink-0 accent-brand"
                />
                <span className="text-ink-muted">
                  I am authorized to operate this device and agree to the{" "}
                  <a href="/terms" target="_blank" rel="noopener" className="text-brand underline">
                    Terms
                  </a>{" "}
                  and{" "}
                  <a href="/privacy" target="_blank" rel="noopener" className="text-brand underline">
                    Privacy Policy
                  </a>
                  .
                </span>
              </label>
              {error && (
                <p role="alert" className="text-sm text-danger">
                  {error}
                </p>
              )}
              <Button type="submit" disabled={busy || !accepted || authLoading} className="w-full">
                {busy && <LoaderCircle size={16} className="animate-spin" />}
                {me ? "Add to my account" : "Sign in & add this device"}
              </Button>
              {!me && !authLoading && (
                <p className="-mt-2 text-center text-xs text-ink-muted">
                  You'll come straight back here after signing in.
                </p>
              )}
            </form>
          </Card>
        ) : (
          <Card className="p-6 sm:p-8">
            <h2 className="font-display text-base font-bold text-ink">Enter what your device shows</h2>
            <p className="mt-1 text-sm leading-6 text-ink-muted">
              Install and open abacad on the device you want to add. It displays a device ID and a
              claim code — type both here.
            </p>
            <form onSubmit={submitLookup} className="mt-5 flex flex-col gap-4">
              <div className="flex flex-col gap-2">
                <Label htmlFor="claim-id">Device ID</Label>
                <Input
                  id="claim-id"
                  autoFocus
                  value={idInput}
                  onChange={(event) => setIdInput(event.target.value)}
                  placeholder="ktphznmqvxbdfgwr"
                  className="text-center font-mono text-base lowercase tracking-[0.15em]"
                />
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="claim-code">Claim code</Label>
                <Input
                  id="claim-code"
                  value={codeInput}
                  onChange={(event) => setCodeInput(event.target.value)}
                  placeholder="WXYZ-2K7M"
                  className="text-center font-mono text-lg uppercase tracking-[0.3em]"
                />
              </div>
              {error && (
                <p role="alert" className="text-sm text-danger">
                  {error}
                </p>
              )}
              <Button type="submit" className="w-full">
                Continue
              </Button>
              <p className="text-center text-xs leading-5 text-ink-muted">
                Don't have the app yet?{" "}
                <a href="/downloads" className="text-brand underline">
                  Download it
                </a>
                .
              </p>
            </form>
          </Card>
        )}
      </div>
    </div>
  );
}
