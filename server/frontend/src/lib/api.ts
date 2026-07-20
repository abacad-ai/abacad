// Thin typed client for the dashboard API. Same-origin (cookies included); the
// Vite dev proxy forwards to Go in development.

export interface DeviceView {
  id: string;
  name: string;
  online: boolean;
  platform?: string; // e.g. "android", "macos"; blank if unset
  last_seen?: string;
  created_at: string;
  ssh_host?: string; // ssh <ssh_host> reaches this device via the jump host
  screenshot_at?: number; // unix seconds of the last stored screenshot; absent if none
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

// One published client build, as found on the server's downloads directory.
export interface ClientBuild {
  platform: string; // "macos", "android", …
  file: string;
  url: string; // /downloads/<file>
  size: number; // bytes
  updated_at: number; // unix seconds
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

export const api = {
  me: () => req<Me>("/api/auth/me"),
  authConfig: () => req<AuthConfig>("/api/auth/config"),
  register: (email: string, password: string) =>
    req<Me>("/api/auth/register", { method: "POST", body: JSON.stringify({ email, password }) }),
  login: (email: string, password: string) =>
    req<Me>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => req<void>("/api/auth/logout", { method: "POST" }),

  // Public (no session): the downloads page works signed out.
  downloads: () => req<ClientBuild[]>("/api/downloads"),

  devices: () => req<DeviceView[]>("/api/devices"),
  device: (id: string) => req<DeviceView>(`/api/devices/${id}`),
  createDevice: (name: string, platform?: string) =>
    req<NewDevice>("/api/devices", { method: "POST", body: JSON.stringify({ name, platform }) }),
  renameDevice: (id: string, name: string) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ name }) }),
  deleteDevice: (id: string) => req<void>(`/api/devices/${id}`, { method: "DELETE" }),
  rotateDeviceToken: (id: string) =>
    req<{ device_token: string; wss_url: string }>(`/api/devices/${id}/rotate-token`, { method: "POST" }),
  deviceScreenshotUrl: (id: string) => `/api/devices/${id}/screenshot`,
  deviceEvents: (id: string) => req<DeviceEvents>(`/api/devices/${id}/events`),

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
