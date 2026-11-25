import type { Device } from "@/lib/types"
import { deviceStatus } from "@/lib/device"
import { cn } from "@/lib/utils"

/**
 * A device's state as one dot. There is no probe: blue means a live session went
 * through the hub — the same blue as the front pane, the one you are looking at.
 * Grey is silence, coral is the last attempt failing.
 */
export function StatusDot({
  device,
  halo = false,
  className,
}: {
  device: Device
  halo?: boolean
  className?: string
}) {
  const status = deviceStatus(device)
  return (
    <span
      aria-hidden="true"
      className={cn(
        "size-1.5 shrink-0 rounded-full",
        status === "online" && "bg-brand",
        status === "attention" && "bg-signal",
        status === "stale" && "bg-faint",
        status === "unknown" && "border border-faint",
        halo && status === "online" && "shadow-[0_0_0_3px_var(--brand-halo)]",
        className,
      )}
    />
  )
}
