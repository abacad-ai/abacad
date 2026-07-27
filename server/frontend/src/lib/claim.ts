// A claim in flight, parked across the sign-in / sign-up detour.
//
// The /claim page is public so a visitor can confirm which device they hold
// before creating an account. Completing the claim then needs a session, and
// getting one can mean a full cross-origin round trip through Google. Neither
// react-router state nor component state survives that, so the pending claim is
// stashed in sessionStorage: same tab, cleared when the tab closes, and it does
// survive a top-level cross-origin navigation.
//
// The server carries the same intent independently via ?next= on the OAuth
// start, so a browser that blocks storage still lands back on /claim — it just
// has to retype the code. Belt and braces, deliberately.

const KEY = "abacad.pendingClaim";

export interface PendingClaim {
  deviceId: string;
  claimCode: string;
}

export function savePendingClaim(claim: PendingClaim): void {
  try {
    sessionStorage.setItem(KEY, JSON.stringify(claim));
  } catch {
    // Private mode or storage disabled — the ?next= path still works.
  }
}

export function takePendingClaim(): PendingClaim | null {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return null;
    sessionStorage.removeItem(KEY); // single use: consuming it prevents a stale replay
    const parsed = JSON.parse(raw) as Partial<PendingClaim>;
    if (!parsed.deviceId || !parsed.claimCode) return null;
    return { deviceId: parsed.deviceId, claimCode: parsed.claimCode };
  } catch {
    return null;
  }
}

// claimPath builds the /claim URL that restores a pending claim, used as the
// post-login destination.
export function claimPath(claim: PendingClaim): string {
  const p = new URLSearchParams({ d: claim.deviceId, c: claim.claimCode });
  return `/claim?${p.toString()}`;
}

// normalizeClaimCode mirrors the server's tolerance: case, spaces and the group
// dash are all optional, and the canonical form is XXXX-XXXX. Returns "" when
// the input isn't eight alphanumerics, so the UI can keep the button disabled.
export function normalizeClaimCode(input: string): string {
  const c = input.toUpperCase().replace(/[^A-Z0-9]/g, "");
  if (c.length !== 8) return "";
  return `${c.slice(0, 4)}-${c.slice(4)}`;
}

// normalizeDeviceId matches auth.NewDeviceID: exactly 16 lowercase letters.
export function normalizeDeviceId(input: string): string {
  const id = input.trim().toLowerCase().replace(/[^a-z]/g, "");
  return /^[a-z]{16}$/.test(id) ? id : "";
}
