import { ChevronRight, LogOut, Plus } from "lucide-react"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { deviceStatus, statusLabel } from "@/lib/device"
import { PlatformIcon } from "@/lib/platform"
import type { Device } from "@/lib/types"
import { cn } from "@/lib/utils"

function StatusDot({ device, className }: { device: Device; className?: string }) {
  const status = deviceStatus(device)
  return (
    <span
      className={cn(
        "size-1.5 shrink-0 rounded-full",
        status === "online" && "bg-[#1f9d7f] shadow-[0_0_0_3px_rgba(31,157,127,0.12)]",
        status === "attention" && "bg-[#b4752a]",
        status === "stale" && "bg-[#b3b4aa]",
        status === "unknown" && "border border-[#b3b4aa]",
        className,
      )}
    />
  )
}

function RailItem({ device, selected }: { device: Device; selected: boolean }) {
  const status = deviceStatus(device)
  return (
    <Link
      to={`/devices/${encodeURIComponent(device.id)}`}
      aria-current={selected ? "page" : undefined}
      title={`${device.name} — ${device.mode === "direct" ? "catalog only" : statusLabel(status)}`}
      className={cn(
        "group flex min-w-[168px] items-center gap-2.5 rounded-[10px] px-2.5 py-2.5 transition-colors lg:min-w-0 lg:px-2",
        selected
          ? "bg-[#e7f1ec] shadow-[inset_2px_0_0_var(--primary)]"
          : "hover:bg-[#f1ebdf]",
      )}
    >
      <StatusDot device={device} />
      <span
        className={cn(
          "grid size-6 shrink-0 place-items-center rounded-md bg-[#ece6da] text-muted-foreground",
          selected && "bg-[#d3e8df] text-[#0d4a42]",
        )}
      >
        <PlatformIcon platform={device.platform} className="size-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span
          className={cn(
            "block truncate text-[12.5px] leading-tight font-medium text-[#4b4d46]",
            selected && "text-foreground",
          )}
        >
          {device.name}
        </span>
        <span className="mt-0.5 block truncate font-mono text-[9.5px] text-muted-foreground/80">
          {device.host}
        </span>
      </span>
      {selected ? <ChevronRight className="size-3 shrink-0 text-muted-foreground" /> : null}
    </Link>
  )
}

export function ConsoleRail({
  devices,
  selectedId,
  hubOnline,
  version,
  onAdd,
  onLogout,
}: {
  devices: Device[]
  selectedId?: string
  hubOnline: boolean
  version?: string
  onAdd: () => void
  onLogout: () => void
}) {
  return (
    <aside className="flex w-full min-w-0 shrink-0 flex-col border-b border-border/80 bg-[#f8f4eb] lg:h-screen lg:w-[296px] lg:border-r lg:border-b-0">
      <div className="flex h-16 items-center justify-between px-4 lg:px-3.5">
        <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
          <span className="grid size-8 place-items-center rounded-[10px] bg-primary text-[17px] font-semibold text-primary-foreground shadow-sm">
            a
          </span>
          <span className="text-[15px] font-semibold tracking-tight">abacad</span>
          <span
            className={cn(
              "ml-0.5 size-1.5 rounded-full lg:hidden",
              hubOnline ? "bg-[#1f9d7f]" : "bg-[#c05a4f]",
            )}
            title={hubOnline ? "Hub online" : "Hub offline"}
          />
        </Link>
        <div className="flex items-center gap-1 lg:hidden">
          <Button variant="ghost" size="icon-sm" onClick={onAdd} aria-label="Add device">
            <Plus />
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={onLogout} aria-label="Sign out">
            <LogOut />
          </Button>
        </div>
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-visible lg:overflow-y-auto">
        <div className="flex items-center justify-between px-4 pb-2 lg:px-5">
          <span className="text-[9.5px] font-semibold tracking-[0.16em] text-muted-foreground/70 uppercase">
            Machines
          </span>
          <span className="text-[10px] tabular-nums text-muted-foreground/60">{devices.length}</span>
        </div>

        <nav className="flex w-full min-w-0 gap-1.5 overflow-x-auto px-3 pb-3 lg:flex-col lg:gap-0.5 lg:overflow-visible">
          {devices.map((device) => (
            <RailItem key={device.id} device={device} selected={device.id === selectedId} />
          ))}
        </nav>

        <div className="hidden px-3 lg:block">
          <Button
            variant="outline"
            className="h-9 w-full justify-center gap-1.5 border-dashed border-border/90 bg-transparent text-[11.5px] font-normal text-muted-foreground shadow-none hover:bg-[#f1ebdf] hover:text-foreground"
            onClick={onAdd}
          >
            <Plus className="size-3.5" />
            Add device
          </Button>
        </div>

      </div>

      <div className="hidden border-t border-border/70 px-4 py-3.5 lg:block">
        <div className="flex items-center gap-1.5 text-[10.5px] text-muted-foreground">
          <span
            className={cn("size-1.5 rounded-full", hubOnline ? "bg-[#1f9d7f]" : "bg-[#c05a4f]")}
          />
          <span>{hubOnline ? "hub online" : "hub offline"}</span>
          {version ? (
            <span className="ml-auto font-mono text-[9.5px] text-muted-foreground/60">v{version}</span>
          ) : null}
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="-ml-2 mt-1 h-7 px-2 text-[11px] font-normal text-muted-foreground hover:text-foreground"
          onClick={onLogout}
        >
          <LogOut className="size-3.5" />
          Sign out
        </Button>
      </div>
    </aside>
  )
}
