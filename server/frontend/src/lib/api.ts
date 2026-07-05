// Thin typed client for the dashboard API. Same-origin (cookies included); the
// Vite dev proxy forwards to Go in development.

export interface DeviceView {
  id: string;
  name: string;
  online: boolean;
  last_seen?: string;
  created_at: string;
}

export interface NewDevice {
  id: string;
  name: string;
  device_token: string;
  wss_url: string;
}

export interface McpTokenInfo {
  exists: boolean;
  created_at?: string;
  last_used?: string;
}

export interface Me {
  account_id: string;
  email: string;
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
  register: (email: string, password: string) =>
    req<Me>("/api/auth/register", { method: "POST", body: JSON.stringify({ email, password }) }),
  login: (email: string, password: string) =>
    req<Me>("/api/auth/login", { method: "POST", body: JSON.stringify({ email, password }) }),
  logout: () => req<void>("/api/auth/logout", { method: "POST" }),

  devices: () => req<DeviceView[]>("/api/devices"),
  createDevice: (name: string) =>
    req<NewDevice>("/api/devices", { method: "POST", body: JSON.stringify({ name }) }),
  renameDevice: (id: string, name: string) =>
    req<void>(`/api/devices/${id}`, { method: "PATCH", body: JSON.stringify({ name }) }),
  deleteDevice: (id: string) => req<void>(`/api/devices/${id}`, { method: "DELETE" }),
  rotateDeviceToken: (id: string) =>
    req<{ device_token: string; wss_url: string }>(`/api/devices/${id}/rotate-token`, { method: "POST" }),
  deviceScreenshotUrl: (id: string) => `/api/devices/${id}/screenshot`,
  deviceEvents: (id: string) => req<DeviceEvents>(`/api/devices/${id}/events`),

  mcpToken: () => req<McpTokenInfo>("/api/mcp-token"),
  rotateMcpToken: () => req<{ mcp_token: string; mcp_url: string }>("/api/mcp-token/rotate", { method: "POST" }),
};
