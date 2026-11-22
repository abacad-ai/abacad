import {
  Copy,
  ExternalLink,
  KeyRound,
  Pencil,
  RefreshCw,
  Trash2,
} from "lucide-react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { cn } from "@/lib/utils"
import { PlatformIcon, platformLabel } from "@/lib/platform"
import type { Device } from "@/lib/types"

export function DeviceCard({
  device,
  onEdit,
  onProbe,
  onRemove,
  onCopyVNC,
  onCopyToken,
}: {
  device: Device
  onEdit: (device: Device) => void
  onProbe: (id: string) => void
  onRemove: (device: Device) => void
  onCopyVNC: (device: Device) => void
  onCopyToken: (device: Device) => void
}) {
  const mode = device.mode || "gateway"
  const signIn = device.auth ?? "vnc"
  const port = device.vnc_port ?? 0
  const canOpen = mode !== "direct"
  const usesToken = mode === "gateway" && Boolean(device.token)

  const chips: string[] = []
  chips.push(port ? `VNC :${port}` : "browser only")
  if (device.banner) chips.push(device.banner)
  if (mode === "l4") chips.push("L4 · device password")
  if (mode === "direct") chips.push("catalog only")
  if (signIn === "ard") {
    chips.push(`macOS account${device.username ? ` · ${device.username}` : ""}`)
  }

  return (
    <Card className="gap-4 border-border/80 bg-card/90 shadow-sm shadow-foreground/5 transition-shadow hover:shadow-md">
      <CardHeader className="flex-row items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-muted-foreground">
          <PlatformIcon platform={device.platform} className="size-4" />
          <span className="text-[11px] font-medium tracking-[0.14em] uppercase">
            {platformLabel(device.platform)}
          </span>
        </div>
        <span
          className={cn(
            "inline-flex items-center gap-1.5 text-xs font-medium",
            device.online ? "text-emerald-700" : "text-muted-foreground",
          )}
        >
          <span
            className={cn(
              "size-1.5 rounded-full",
              device.online ? "bg-emerald-500" : "bg-muted-foreground/40",
            )}
          />
          {device.online ? "online" : "offline"}
        </span>
      </CardHeader>

      <CardContent className="space-y-3">
        <div className="space-y-1">
          <CardTitle className="text-xl tracking-tight">{device.name}</CardTitle>
          <p className="font-mono text-sm text-muted-foreground">
            {device.host}:{device.port}
          </p>
        </div>

        <div className="flex flex-wrap gap-1.5">
          {chips.map((chip) => (
            <Badge
              key={chip}
              variant="outline"
              className="h-6 max-w-full truncate font-normal text-muted-foreground"
            >
              {chip}
            </Badge>
          ))}
        </div>

        {device.warning ? (
          <p className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-relaxed text-amber-900">
            {device.warning}
          </p>
        ) : null}

        {device.last_error ? (
          <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs leading-relaxed break-words text-red-800">
            {device.last_error}
          </p>
        ) : null}
      </CardContent>

      <CardFooter className="flex flex-wrap gap-1.5 border-border/70 bg-transparent pt-3">
        {canOpen ? (
          <Button asChild size="sm">
            <a href={`/vnc/${encodeURIComponent(device.id)}`} target="_blank" rel="noopener">
              <ExternalLink />
              Open
            </a>
          </Button>
        ) : null}
        {port ? (
          <Button size="sm" variant="outline" onClick={() => onCopyVNC(device)}>
            <Copy />
            Copy VNC
          </Button>
        ) : null}
        <Button size="sm" variant="ghost" onClick={() => onEdit(device)}>
          <Pencil />
          Edit
        </Button>
        <Button size="sm" variant="ghost" onClick={() => onProbe(device.id)}>
          <RefreshCw />
          Check
        </Button>
        <Button
          size="sm"
          variant="ghost"
          className="text-muted-foreground hover:text-destructive"
          onClick={() => onRemove(device)}
        >
          <Trash2 />
          Remove
        </Button>
      </CardFooter>

      {usesToken && port > 0 ? (
        <CardContent className="pt-0">
          <div className="flex items-center gap-2 rounded-lg border border-border/70 bg-muted/40 px-2.5 py-1.5">
            <KeyRound className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
              token
            </span>
            <code className="min-w-0 flex-1 truncate font-mono text-xs">{device.token}</code>
            <Button
              size="icon-xs"
              variant="ghost"
              aria-label="Copy token"
              title="Copy token"
              onClick={() => onCopyToken(device)}
            >
              <Copy className="size-3.5" />
            </Button>
          </div>
        </CardContent>
      ) : null}
    </Card>
  )
}
