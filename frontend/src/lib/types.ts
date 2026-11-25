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
  created_at?: string
  online: boolean
  known?: boolean
  last_seen?: string
  last_error?: string
}

export interface HubInfo {
  name: string
  version: string
  listen: string
  devices: number
}

export interface DevicesResponse {
  devices: Device[]
  hub: { listen: string }
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
}
