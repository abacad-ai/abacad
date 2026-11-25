import type {
  DevicePayload,
  DeviceResponse,
  DevicesResponse,
  HubInfo,
  SessionResponse,
} from "@/lib/types"

export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  })

  let body: unknown = null
  try {
    body = await response.json()
  } catch {
    // The API always returns JSON, but a proxy error may not.
  }

  if (!response.ok) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : `HTTP ${response.status}`
    throw new ApiError(response.status, message)
  }

  return body as T
}

export const api = {
  session: () => request<SessionResponse>("/api/session"),
  login: (token: string) =>
    request<{ ok: boolean }>("/api/login", {
      method: "POST",
      body: JSON.stringify({ token }),
    }),
  logout: () => request<{ ok: boolean }>("/api/logout", { method: "POST" }),
  info: () => request<HubInfo>("/api/info"),
  devices: () => request<DevicesResponse>("/api/devices"),
  // The admin token, for baking into the agent skill. Requires a signed-in session.
  token: () => request<{ token: string }>("/api/token"),
  addDevice: (payload: DevicePayload) =>
    request<DeviceResponse>("/api/devices", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  updateDevice: (id: string, payload: Partial<DevicePayload>) =>
    request<DeviceResponse>(`/api/devices/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),
  removeDevice: (id: string) =>
    request<{ ok: boolean }>(`/api/devices/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
}
