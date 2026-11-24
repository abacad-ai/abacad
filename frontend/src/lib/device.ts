import type { Device } from "@/lib/types"

export type DeviceStatus = "online" | "attention" | "stale" | "unknown"

export function deviceStatus(device: Device): DeviceStatus {
  if (device.online) return "online"
  if (device.last_error) return "attention"
  if (device.known) return "stale"
  return "unknown"
}

export function statusLabel(status: DeviceStatus): string {
  if (status === "online") return "online"
  if (status === "attention") return "failed"
  if (status === "stale") return "last seen"
  return "not seen"
}
