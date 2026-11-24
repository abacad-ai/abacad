import { ExternalLink, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
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

function RailItem({
  device,
  selected,
  onEdit,
  onRemove,
}: {
  device: Device
  selected: boolean
  onEdit: () => void
  onRemove: () => void
}) {
  const status = deviceStatus(device)
  return (
    <div
      className={cn(
        "group relative flex min-w-[172px] items-center rounded-[10px] transition-colors lg:min-w-0",
        selected ? "bg-[#e7f1ec] shadow-[inset_2px_0_0_var(--primary)]" : "hover:bg-[#f1ebdf]",
      )}
    >
      <Link
        to={`/devices/${encodeURIComponent(device.id)}`}
        aria-current={selected ? "page" : undefined}
        title={`${device.name} — ${device.mode === "direct" ? "catalog only" : statusLabel(status)}`}
        className="flex min-w-0 flex-1 items-center gap-2.5 py-2.5 pr-8 pl-2.5 lg:pl-2"
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
      </Link>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={`Actions for ${device.name}`}
            className={cn(
              "absolute top-1/2 right-1 -translate-y-1/2 text-muted-foreground/70 hover:bg-transparent hover:text-foreground focus-visible:opacity-100 data-[state=open]:opacity-100",
              selected
                ? "opacity-60"
                : "opacity-60 lg:opacity-0 lg:group-hover:opacity-100 lg:group-focus-within:opacity-100",
            )}
          >
            <MoreHorizontal className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-[168px]">
          {device.mode === "direct" ? null : (
            <DropdownMenuItem asChild>
              <Link to={`/vnc/${encodeURIComponent(device.id)}`} target="_blank" rel="noreferrer">
                <ExternalLink />
                Open in new tab
              </Link>
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onSelect={onEdit}>
            <Pencil />
            Edit
          </DropdownMenuItem>
          <DropdownMenuItem variant="destructive" onSelect={onRemove}>
            <Trash2 />
            Remove
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

export function ConsoleRail({
  devices,
  selectedId,
  onAdd,
  onEdit,
  onRemove,
}: {
  devices: Device[]
  selectedId?: string
  onAdd: () => void
  onEdit: (device: Device) => void
  onRemove: (device: Device) => void
}) {
  return (
    <aside className="flex w-full min-w-0 shrink-0 flex-col border-b border-border/80 bg-[#f8f4eb] lg:h-screen lg:w-[288px] lg:border-r lg:border-b-0">
      <div className="flex h-16 shrink-0 items-center justify-between px-4 lg:px-3.5">
        <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
          <span className="grid size-8 place-items-center rounded-[10px] bg-primary text-[17px] font-semibold text-primary-foreground shadow-sm">
            a
          </span>
          <span className="text-[15px] font-semibold tracking-tight">abacad</span>
        </Link>
        <Button variant="ghost" size="icon-sm" className="lg:hidden" onClick={onAdd} aria-label="Add device">
          <Plus />
        </Button>
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-visible lg:overflow-y-auto">
        <nav
          aria-label="Machines"
          className="flex w-full min-w-0 gap-1.5 overflow-x-auto px-3 pb-3 lg:flex-col lg:gap-0.5 lg:overflow-visible lg:pb-4"
        >
          {devices.map((device) => (
            <RailItem
              key={device.id}
              device={device}
              selected={device.id === selectedId}
              onEdit={() => onEdit(device)}
              onRemove={() => onRemove(device)}
            />
          ))}
        </nav>
      </div>

      <div className="hidden shrink-0 px-3 pb-4 lg:block">
        <Button
          variant="outline"
          className="h-9 w-full justify-center gap-1.5 border-dashed border-border/90 bg-transparent text-[11.5px] font-normal text-muted-foreground shadow-none hover:bg-[#f1ebdf] hover:text-foreground"
          onClick={onAdd}
        >
          <Plus className="size-3.5" />
          Add device
        </Button>
      </div>
    </aside>
  )
}
