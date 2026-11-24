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

export function relativeTime(iso?: string): string {
  if (!iso) return "never"
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return "never"
  const seconds = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (seconds < 10) return "just now"
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return shortDate(iso)
}

export function shortDate(iso?: string): string {
  if (!iso) return "—"
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return "—"
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  }).format(date)
}

export function modeLabel(mode: Device["mode"]): string {
  if (mode === "l4") return "Raw TCP"
  if (mode === "direct") return "Catalog only"
  return "Gateway"
}

export function modeDescription(device: Device): string {
  if (device.mode === "l4") return "Browser via hub · the device asks for its own password"
  if (device.mode === "direct") return "Catalog only · no traffic through the hub"
  return "Browser via hub · the password stays here"
}

export function signInLabel(device: Device): string {
  if (device.auth === "ard") {
    return device.username ? `macOS account · ${device.username}` : "macOS account"
  }
  return "VNC password"
}
