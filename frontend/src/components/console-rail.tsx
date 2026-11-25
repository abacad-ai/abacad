import { Bot, ExternalLink, MoreHorizontal, Pencil, Plus, Trash2 } from "lucide-react"
import { Link } from "react-router-dom"

import { BrandMark } from "@/components/brand-mark"
import { StatusDot } from "@/components/status-dot"
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
  return (
    <div
      className={cn(
        "group relative flex min-w-[176px] items-center rounded-[10px] transition-colors lg:min-w-0",
        selected
          ? "bg-card text-foreground shadow-card ring-1 ring-border"
          : "hover:bg-foreground/[0.04]",
      )}
    >
      <Link
        to={`/devices/${encodeURIComponent(device.id)}`}
        aria-current={selected ? "page" : undefined}
        title={`${device.name} — ${device.mode === "direct" ? "catalog only" : statusLabel(deviceStatus(device))}`}
        className="flex min-w-0 flex-1 items-center gap-2.5 py-2 pr-8 pl-2.5"
      >
        <StatusDot device={device} halo />
        <span
          className={cn(
            "grid size-6 shrink-0 place-items-center rounded-[6px]",
            selected
              ? "bg-foreground text-background"
              : "bg-foreground/[0.055] text-muted-foreground",
          )}
        >
          <PlatformIcon platform={device.platform} className="size-3.5" />
        </span>
        <span className="min-w-0 flex-1">
          <span
            className={cn(
              "block truncate text-[12.5px] leading-tight font-medium",
              selected ? "text-foreground" : "text-foreground/80",
            )}
          >
            {device.name}
          </span>
          <span className="mt-1 block truncate font-mono text-[10px] text-muted-foreground">
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
  skillActive,
  onAdd,
  onEdit,
  onRemove,
}: {
  devices: Device[]
  selectedId?: string
  skillActive?: boolean
  onAdd: () => void
  onEdit: (device: Device) => void
  onRemove: (device: Device) => void
}) {
  return (
    <aside className="flex w-full min-w-0 shrink-0 flex-col border-b border-border bg-sidebar lg:h-screen lg:w-[288px] lg:border-r lg:border-b-0">
      <div className="flex h-16 shrink-0 items-center justify-between px-4 lg:px-3.5">
        <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
          <BrandMark className="size-7 shrink-0" />
          <span className="text-[15px] font-semibold tracking-[-0.035em]">abacad</span>
        </Link>
        <div className="flex items-center gap-1 lg:hidden">
          <Button asChild variant="ghost" size="icon-sm" aria-label="Agent skill">
            <Link to="/skill">
              <Bot />
            </Link>
          </Button>
          <Button variant="ghost" size="icon-sm" onClick={onAdd} aria-label="Add device">
            <Plus />
          </Button>
        </div>
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-visible lg:overflow-y-auto">
        <nav
          aria-label="Machines"
          className="flex w-full min-w-0 gap-1.5 overflow-x-auto px-3 pb-3 lg:flex-col lg:gap-1 lg:overflow-visible lg:pb-4"
        >
          {devices.length === 0 ? (
            <p className="px-2.5 py-2 text-[11.5px] text-muted-foreground">None configured</p>
          ) : null}
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

      <div className="hidden shrink-0 flex-col gap-1.5 px-3 pb-4 lg:flex">
        <Button
          asChild
          variant="ghost"
          className={cn(
            "h-8 w-full justify-start gap-2 px-2 text-[12px] font-normal",
            skillActive
              ? "bg-card text-foreground shadow-card ring-1 ring-border hover:bg-card"
              : "text-muted-foreground hover:bg-foreground/[0.04] hover:text-foreground",
          )}
        >
          <Link to="/skill" aria-current={skillActive ? "page" : undefined}>
            <Bot className="size-3.5" />
            Skill for agents
          </Link>
        </Button>
        <Button
          variant="outline"
          className="h-9 w-full justify-center gap-1.5 text-[12px] font-normal text-muted-foreground hover:text-foreground"
          onClick={onAdd}
        >
          <Plus className="size-3.5" />
          Add device
        </Button>
      </div>
    </aside>
  )
}
