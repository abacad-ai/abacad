import { useMemo, type ReactNode, type Ref } from "react"

import type { Device } from "@/lib/types"
import { cn } from "@/lib/utils"

// The screen is noVNC's own application, vendored under /novnc and loaded in an
// iframe — the same split Proxmox VE uses. Its control bar already carries the
// keyboard, clipboard, fullscreen, scaling and reconnect controls, so the hub
// only hands it the WebSocket path and gets out of the way.
export function vncAppUrl(id: string, nonce = 0) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  const params = new URLSearchParams({
    autoconnect: "1",
    path: `${protocol}//${window.location.host}/ws/vnc/${encodeURIComponent(id)}`,
    resize: "scale",
    reconnect: "1",
    reconnect_delay: "1500",
    quality: "7",
    compression: "1",
    show_dot: "1",
    view_only: "0",
    // Reloading the frame with the same URL would keep the old session; the
    // nonce makes the browser fetch it again.
    _r: String(nonce),
  })
  return `/novnc/vnc.html?${params.toString()}`
}

export function VncFrame({
  device,
  nonce = 0,
  className,
  frameRef,
  children,
}: {
  device: Device
  nonce?: number
  className?: string
  /** The iframe itself, for overlays that need to reach into the noVNC app. */
  frameRef?: Ref<HTMLIFrameElement>
  /** Painted above the screen — the device's own buttons, for one. */
  children?: ReactNode
}) {
  const src = useMemo(() => vncAppUrl(device.id, nonce), [device.id, nonce])

  return (
    <div className={cn("relative h-full w-full bg-screen", className)}>
      <iframe
        ref={frameRef}
        src={src}
        title={`${device.name} screen`}
        className="absolute inset-0 h-full w-full border-0"
        allow="clipboard-read; clipboard-write; fullscreen"
      />
      {children}
    </div>
  )
}
