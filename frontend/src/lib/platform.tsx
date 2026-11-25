import Android from "@thesvg/react/android"
import Apple from "@thesvg/react/apple"
import Linux from "@thesvg/react/linux"
import Windows from "@thesvg/react/windows"
import { Monitor } from "lucide-react"
import type { ReactNode } from "react"

import { cn } from "@/lib/utils"

const labels: Record<string, string> = {
  macos: "macOS",
  linux: "Linux",
  windows: "Windows",
  android: "Android",
  ios: "iOS",
  ipados: "iPadOS",
}

// Brand marks from thesvg.org, in their mono variants so they follow
// currentColor like the rest of the UI. thesvg's ios / macos entries are
// wordmarks, unreadable at icon size, so Apple platforms share the apple mark.
// Windows has no mono variant and hard-codes its fill on the path, hence the
// fill-current override.
const icons: Record<string, (className?: string) => ReactNode> = {
  macos: (className) => <Apple variant="mono" className={className} />,
  ios: (className) => <Apple variant="mono" className={className} />,
  ipados: (className) => <Apple variant="mono" className={className} />,
  linux: (className) => <Linux variant="mono" className={className} />,
  android: (className) => <Android variant="mono" className={className} />,
  windows: (className) => <Windows className={cn("[&_path]:fill-current", className)} />,
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
  const icon = icons[platform ?? ""]
  return icon ? icon(className) : <Monitor className={className} />
}
