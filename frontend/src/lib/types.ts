export type SignIn = "vnc" | "ard"
export type DeviceMode = "gateway" | "l4" | "direct"

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
  banner?: string
  last_seen?: string
  warning?: string
  last_error?: string
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
}
