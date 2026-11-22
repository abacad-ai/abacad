export type SignIn = "vnc" | "ard"
export type DeviceMode = "gateway" | "l4" | "direct"
export type HealthMode = "session" | "command" | "vnc"

export interface Device {
  id: string
  name: string
  host: string
  port: number
  username?: string
  auth?: SignIn
  platform?: string
  mode: DeviceMode
  vnc_port?: number
  token?: string
  created_at?: string
  online: boolean
  known?: boolean
  banner?: string
  detail?: string
  last_seen?: string
  warning?: string
  last_error?: string
  next_check?: string
  health?: HealthMode
  health_command?: string
}

export interface DiscoveredDevice {
  name?: string
  host: string
  port: number
}

export interface HubInfo {
  name: string
  version: string
  listen: string
  mdns: boolean
  devices: number
}

export interface DevicesResponse {
  devices: Device[]
  hub: { listen: string }
}

export interface DiscoveredResponse {
  devices: DiscoveredDevice[]
}

export interface SessionResponse {
  authenticated: boolean
}

export interface DeviceResponse {
  device: Device
}

export interface DevicePayload {
  name: string
  host: string
  port: number
  auth: SignIn
  username: string
  password?: string
  platform: string
  mode: DeviceMode
  expose: boolean
  health: HealthMode
  health_command: string
}
