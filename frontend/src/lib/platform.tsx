import { Laptop, Monitor, Smartphone, Terminal } from "lucide-react"
import type { ComponentType } from "react"

const labels: Record<string, string> = {
  macos: "macOS",
  linux: "Linux",
  windows: "Windows",
  android: "Android",
}

const icons: Record<string, ComponentType<{ className?: string }>> = {
  macos: Laptop,
  linux: Terminal,
  windows: Monitor,
  android: Smartphone,
}

export function platformLabel(platform?: string) {
  return labels[platform ?? ""] ?? "Device"
}

export function PlatformIcon({
  platform,
  className,
}: {
  platform?: string
  className?: string
}) {
  const Icon = icons[platform ?? ""] ?? Monitor
  return <Icon className={className} />
}
