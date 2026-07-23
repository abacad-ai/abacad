// Thin typed client for the dashboard API. Same-origin (cookies included); the
// Vite dev proxy forwards to Go in development.

export interface DeviceView {
  id: string;
  name: string;
  online: boolean;
  activity?: "active" | "asleep"; // present only when online; "asleep" = screen off but reachable
  platform?: string; // e.g. "android", "macos"; blank if unset
  version?: string; // client version reported on connect; blank if unknown
  last_seen?: string;
  created_at: string;
  ssh_host?: string; // ssh <ssh_host> reaches this device via the jump host
  screenshot_at?: number; // unix seconds of the last stored screenshot; absent if none
  humanize: boolean; // smooth pointer motion on this device; default off, opt-in with attestation
  expires_at?: string; // enrollment expiry (ISO); absent = permanent (never expires)
}

export interface SshKey {
  id: string;
  name: string;
  fingerprint: string;
  public_key: string;
  created_at: string;
  last_used?: string;
}

export interface NewDevice {
  id: string;
  name: string;
  device_token: string;
  wss_url: string;
  browser_url?: string; // set for browser devices: https://<id>.<base-domain>
}

// A key's capability envelope. all_devices / all_methods are wildcards that also
// cover devices/methods added in the future — distinct from listing every current
// one. When a wildcard is true its companion list is ignored.
export interface KeyScope {
  all_devices: boolean;
  device_ids: string[];
  all_methods: boolean;
  methods: string[];
  allow_tunnel: boolean;
}

export interface ApiKey extends KeyScope {
  id: string;
  name: string;
  created_at: string;
  last_used?: string;
}

// The create-key response: the secret is returned exactly once.
export interface NewApiKey {
  secret: string;
  mcp_url: string;
  key: ApiKey;
}

// Create/update payload.
export interface KeyInput {
  name: string;
  all_devices: boolean;
  device_ids: string[];
  all_methods: boolean;
  methods: string[];
  allow_tunnel: boolean;
}

export interface Me {
  account_id: string;
  email: string;
}

// Which optional sign-in methods the server has configured.
export interface AuthConfig {
  google: boolean;
}

// One entry of a device's activity log (mirrors the Go events.Event).
export interface DeviceEvent {
  ts: number; // unix millis
  kind: "connected" | "disconnected" | "command";
  method?: string;
  source?: string; // agent | dashboard
  duration_ms?: number;
  outcome?: string; // ok | timeout | device_gone | canceled | error
  detail?: string;
}

export interface DeviceEvents {
  online: boolean;
  events: DeviceEvent[];
}

// One row of the account-wide activity trail (mirrors the Go store.Activity).
export interface ActivityItem {
  id: number;
  ts: number; // unix millis
  kind: string; // dotted category.action ("auth.login") or bare "command"
  device_id?: string;
  method?: string;
  source?: string; // agent | dashboard | ssh | tunnel
  outcome?: string;
  duration_ms?: number;
  detail?: string;
}

export interface ActivitiesResult {
  activities: ActivityItem[];
  next_before?: number; // absent once the trail is exhausted
}

// One published client build, as listed in the downloads manifest. Named by the
// repo-wide convention abacad-<version>-<platform>-<arch>.<suffix>.
export interface Build {
  platform: string; // "macos", "android", "linux", "windows"
  arch: string; // "amd64" | "arm64" | "universal"
  version: string;
  file: string;
  url: string; // /downloads/<file>
  size: number; // bytes
  sha256: string; // hex; there for a future in-app auto-updater
}

// The client downloads manifest (server's static /downloads/manifest.json, written
// by `make stage`): the current build per platform+arch. There's no downloads API
// endpoint — the manifest is served straight off the downloads dir.
export interface Manifest {
  version: string;
  generated_at: number; // unix seconds
  builds: Build[];
}

export interface ActivityQuery {
  before?: number;
  device?: string;
  kind?: string; // category prefix ("device") or exact kind
  source?: string;
  limit?: number;
}

async function req<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(opts.headers ?? {}) },
    ...opts,
  });
  const text = await res.text();
  let data: unknown = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = { error: text };
    }
  }
  if (!res.ok) {
    const msg = (data as { error?: string })?.error ?? res.statusText;
    throw new ApiError(msg, res.status);
  }
  return data as T;
}

export class ApiError extends Error {
  constructor(message: string, public status: number) {
    super(message);
  }
}

// A pending `abacad connect` pairing, as shown on the /pair approval page.
export interface PairInfo {
  user_code: string;
  platform: string; // CLI-reported OS, may be ""
  status: string; // "pending" | "approved" | "denied"
}

export const api = {
  me: () => req<Me>("/api/auth/me"),
  authConfig: () => req<AuthConfig>("/api/auth/config"),
  register: (email: string, password: string) =>
    req<Me>("/api/auth/register", { method: "POST", body: JSON.stringify({ email, password }) }),
  login: (email: string, password: string) =>
    req<Me>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => req<void>("/api/auth/logout", { method: "POST" }),

  // Public (no session): the downloads page works signed out. The manifest is a
  // static file; a 404 (nothing staged on a fresh server) is normal, so surface
  // it as an empty manifest rather than an error.
  manifest: (): Promise<Manifest> =>
    req<Manifest>("/downloads/manifest.json").catch((err) => {
      if (err instanceof ApiError && err.status === 404) return { version: "", generated_at: 0, builds: [] };
      throw err;
    }),

  // Public: the running server version, for the footer's server/SPA skew check.
  version: () => req<{ version: string }>("/api/version"),

  devices: () => req<DeviceView[]>("/api/devices"),
  device: (id: string) => req<DeviceView>(`/api/devices/${id}`),
  createDevice: (name: string, platform?: string) =>
    req<NewDevice>("/api/devices", { method: "POST", body: JSON.stringify({ name, platform }) }),
  renameDevice: (id: string, name: string) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ name }) }),
  // Enabling humanize requires attested=true (operator authorization); disabling does not.
  setDeviceHumanize: (id: string, humanize: boolean, attested?: boolean) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ humanize, attested }) }),
  // Reset enrollment expiry to now + TTL (hosted service only).
  extendDevice: (id: string) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ extend: true }) }),
  // Remove enrollment expiry; requires attested=true (operator authorization).
  setDevicePermanent: (id: string, attested: boolean) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ permanent: true, attested }) }),
  deleteDevice: (id: string) => req<void>(`/api/devices/${id}`, { method: "DELETE" }),
  rotateDeviceToken: (id: string) =>
    req<{ device_token: string; wss_url: string }>(`/api/devices/${id}/rotate-token`, { method: "POST" }),
  deviceScreenshotUrl: (id: string) => `/api/devices/${id}/screenshot`,
  deviceEvents: (id: string) => req<DeviceEvents>(`/api/devices/${id}/events`),

  // Live view (VNC). Start mints a one-time viewer ticket and tells the device to
  // start its VNC server + reverse-connect; the browser opens noVNC against
  // watch_path. Stop tears the session down.
  vncStart: (id: string) =>
    req<{ ticket: string; watch_path: string; expires_at: string }>(`/api/devices/${id}/vnc/start`, {
      method: "POST",
    }),
  vncStop: (id: string) => req<{ stopped: boolean }>(`/api/devices/${id}/vnc/stop`, { method: "POST" }),

  // Device-authorization pairing (`abacad connect`). Look up a pending code to
  // show what it authorizes, then approve it into the signed-in account.
  pairLookup: (code: string) => req<PairInfo>(`/api/devices/pair?code=${encodeURIComponent(code)}`),
  // accepted must be true: the operator acknowledges what pairing authorizes.
  pairApprove: (user_code: string, name: string, platform: string | undefined, accepted: boolean) =>
    req<{ status: string }>("/api/devices/pair", {
      method: "POST",
      body: JSON.stringify({ user_code, name, platform, accepted }),
    }),

  activities: (q: ActivityQuery = {}) => {
    const params = new URLSearchParams();
    if (q.before) params.set("before", String(q.before));
    if (q.device) params.set("device", q.device);
    if (q.kind) params.set("kind", q.kind);
    if (q.source) params.set("source", q.source);
    if (q.limit) params.set("limit", String(q.limit));
    const qs = params.toString();
    return req<ActivitiesResult>(`/api/activities${qs ? `?${qs}` : ""}`);
  },

  keys: () => req<ApiKey[]>("/api/keys"),
  createKey: (body: KeyInput) => req<NewApiKey>("/api/keys", { method: "POST", body: JSON.stringify(body) }),
  updateKey: (id: string, body: KeyInput) =>
    req<void>(`/api/keys/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteKey: (id: string) => req<void>(`/api/keys/${id}`, { method: "DELETE" }),

  sshKeys: () => req<SshKey[]>("/api/ssh-keys"),
  addSshKey: (name: string, public_key: string) =>
    req<SshKey>("/api/ssh-keys", { method: "POST", body: JSON.stringify({ name, public_key }) }),
  deleteSshKey: (id: string) => req<void>(`/api/ssh-keys/${id}`, { method: "DELETE" }),
};
