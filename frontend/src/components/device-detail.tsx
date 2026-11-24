import {
  AlertTriangle,
  ArrowUpRight,
  ChevronRight,
  Copy,
  Lock,
  Pencil,
  Trash2,
} from "lucide-react"
import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { copyText } from "@/lib/clipboard"
import {
  deviceStatus,
  modeDescription,
  modeLabel,
  relativeTime,
  shortDate,
  signInLabel,
  statusLabel,
} from "@/lib/device"
import { platformLabel, PlatformIcon } from "@/lib/platform"
import type { Device } from "@/lib/types"
import { cn } from "@/lib/utils"

const tones = {
  online: {
    pill: "border-[#cbe3da] bg-[#eff8f3] text-[#0a5a50]",
    dot: "bg-[#1f9d7f]",
    text: "text-[#0a6152]",
  },
  attention: {
    pill: "border-[#e6d8bf] bg-[#f8efdf] text-[#7c4f1c]",
    dot: "bg-[#b4752a]",
    text: "text-[#7c4f1c]",
  },
  stale: {
    pill: "border-border bg-card text-muted-foreground",
    dot: "bg-[#b3b4aa]",
    text: "text-muted-foreground",
  },
  unknown: {
    pill: "border-border bg-card text-muted-foreground",
    dot: "border border-[#b3b4aa]",
    text: "text-muted-foreground",
  },
} as const

async function copy(value: string, message: string) {
  const copied = await copyText(value)
  if (copied) toast.success(message)
  else toast.error("Could not copy")
}

function Row({
  label,
  children,
  last,
}: {
  label: string
  children: ReactNode
  last?: boolean
}) {
  return (
    <div
      className={cn(
        "grid grid-cols-[104px_minmax(0,1fr)] items-center gap-3 py-2.5",
        !last && "border-b border-border/60",
      )}
    >
      <dt className="text-[10.5px] text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 items-center gap-1.5 text-[12px] text-foreground/80">
        {children}
      </dd>
    </div>
  )
}

function DarkCode({ value, onCopy }: { value: string; onCopy: () => void }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.055] py-2 pr-2 pl-3">
      <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-[#dceee8]">{value}</span>
      <Button
        variant="ghost"
        size="icon-xs"
        className="text-[#e9f4f0]/60 hover:bg-white/10 hover:text-white"
        onClick={onCopy}
        aria-label="Copy"
      >
        <Copy className="size-3.5" />
      </Button>
    </div>
  )
}

function StatStrip({ device }: { device: Device }) {
  const token = device.token
    ? device.token.length > 10
      ? `${device.token.slice(0, 4)}…${device.token.slice(-4)}`
      : device.token
    : "—"
  const stats = [
    {
      label: "Last seen",
      value:
        device.mode === "direct" ? "—" : device.known ? relativeTime(device.last_seen) : "never",
    },
    { label: "Added", value: shortDate(device.created_at) },
    { label: "Mode", value: modeLabel(device.mode) },
    { label: "Token", value: token },
  ]

  return (
    <div className="mt-4 grid grid-cols-2 overflow-hidden rounded-2xl border border-border bg-card/55 lg:grid-cols-4">
      {stats.map((stat, index) => (
        <div
          key={stat.label}
          className={cn(
            "px-4 py-3.5",
            index % 2 === 1 && "border-l border-border/60",
            index >= 2 && "border-t border-border/60 lg:border-t-0",
            index === 2 && "lg:border-l lg:border-border/60",
          )}
        >
          <div className="text-[9.5px] font-semibold tracking-[0.14em] text-muted-foreground/75 uppercase">
            {stat.label}
          </div>
          <div className="mt-1 truncate font-mono text-[11.5px] text-foreground/80">{stat.value}</div>
        </div>
      ))}
    </div>
  )
}

export function DeviceDetail({
  device,
  onEdit,
  onRemove,
}: {
  device: Device
  onEdit: () => void
  onRemove: () => void
}) {
  const status = deviceStatus(device)
  const tone = tones[status]
  const canOpen = device.mode !== "direct"
  const label = device.mode === "direct" ? "catalog only" : statusLabel(status)
  const seen =
    device.mode === "direct"
      ? "no traffic through the hub"
      : !device.known
        ? "open it once to verify"
        : device.last_seen
          ? `${status === "online" ? "seen" : "last seen"} ${relativeTime(device.last_seen)}`
          : status === "online"
            ? "seen just now"
            : "no successful session yet"
  const vncURL = device.vnc_port
    ? `vnc://${window.location.hostname || "127.0.0.1"}:${device.vnc_port}`
    : ""
  const mcpURL = `${window.location.origin}/mcp`
  const directAddress = `${device.host}:${device.port}`
  const agentCopiesToken = device.mode === "gateway" && Boolean(device.token)

  async function copyAgent() {
    if (!vncURL) return
    if (device.mode === "gateway" && device.token) {
      await copy(`${vncURL}\npassword: ${device.token}`, "Endpoint and token copied")
      return
    }
    await copy(vncURL, "VNC endpoint copied")
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <header className="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between gap-3 border-b border-border bg-background/85 px-4 backdrop-blur-md lg:px-8">
        <div className="flex min-w-0 items-center gap-1.5 text-[11.5px] text-muted-foreground">
          <Link to="/" className="shrink-0 transition-colors hover:text-foreground">
            Devices
          </Link>
          <ChevronRight className="size-3 shrink-0" />
          <span className="truncate text-foreground/75">{device.name}</span>
        </div>
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "hidden items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-medium sm:inline-flex",
              tone.pill,
            )}
          >
            <span className={cn("size-1.5 rounded-full", tone.dot)} />
            {label}
          </span>
          {canOpen ? (
            <Button asChild size="lg" className="h-9 gap-1.5 px-3.5 text-[12.5px]">
              <a
                href={`/vnc/${encodeURIComponent(device.id)}`}
                target="_blank"
                rel="noopener"
              >
                <span className="hidden sm:inline">Open in browser</span>
                <span className="sm:hidden">Open</span>
                <ArrowUpRight className="size-3.5" />
              </a>
            </Button>
          ) : null}
        </div>
      </header>

      <div className="mx-auto min-w-0 w-full max-w-[1040px] px-5 py-7 lg:px-10 lg:py-9">
        <p className="flex items-center gap-1.5 text-[10px] font-semibold tracking-[0.18em] text-primary uppercase">
          <PlatformIcon platform={device.platform} className="size-3.5" />
          {platformLabel(device.platform)} · {device.host}:{device.port}
        </p>
        <h1 className="mt-2.5 text-[34px] leading-none font-semibold tracking-[-0.045em] lg:text-[42px]">
          {device.name}
        </h1>
        <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-muted-foreground">
          <span className={cn("inline-flex items-center gap-1.5 font-medium", tone.text)}>
            <span className={cn("size-1.5 rounded-full", tone.dot)} />
            {label}
          </span>
          <span className="text-border">·</span>
          <span>{seen}</span>
        </div>

        {device.last_error ? (
          <div className="mt-5 flex items-start gap-2 rounded-xl border border-[#e6d8bf] bg-[#f8efdf] px-3.5 py-2.5 text-[11.5px] leading-relaxed text-[#7c4f1c]">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            <span>
              <span className="font-medium">Last session failed.</span> {device.last_error}
            </span>
          </div>
        ) : null}

        <div className="mt-7 grid min-w-0 gap-4 lg:grid-cols-[1.18fr_0.92fr]">
          <section className="rounded-2xl border border-border bg-card p-5 shadow-[0_1px_2px_rgba(27,28,26,0.02),0_24px_48px_-42px_rgba(27,28,26,0.5)]">
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-[13px] font-semibold tracking-tight">Connection</h2>
              <span className="font-mono text-[9.5px] text-muted-foreground/80">
                mode · {device.mode}
              </span>
            </div>
            <dl className="mt-1">
              <Row label="Endpoint">
                <span className="truncate font-mono text-[11.5px] text-foreground">
                  {device.host}:{device.port}
                </span>
                <Button
                  variant="ghost"
                  size="icon-xs"
                  className="ml-auto text-muted-foreground/70 hover:text-foreground"
                  onClick={() => void copy(`${device.host}:${device.port}`, "Device address copied")}
                  aria-label="Copy device address"
                >
                  <Copy className="size-3.5" />
                </Button>
              </Row>
              <Row label="Platform">
                <PlatformIcon platform={device.platform} className="size-3.5 text-muted-foreground" />
                {platformLabel(device.platform)}
              </Row>
              <Row label="Sign-in">{signInLabel(device)}</Row>
              <Row label="Access">{modeDescription(device)}</Row>
              <Row label="Agent endpoint" last>
                {device.mode === "direct" ? (
                  <>
                    <span className="truncate font-mono text-[11.5px]">{directAddress}</span>
                    <span className="rounded-[5px] bg-[#f2ecdf] px-1.5 py-0.5 text-[9.5px] text-muted-foreground">
                      direct
                    </span>
                  </>
                ) : vncURL ? (
                  <span className="truncate font-mono text-[11.5px]">{vncURL}</span>
                ) : (
                  <>
                    <span>Browser only</span>
                    <span className="rounded-[5px] bg-[#f2ecdf] px-1.5 py-0.5 text-[9.5px] text-muted-foreground">
                      no raw port
                    </span>
                  </>
                )}
              </Row>
            </dl>
          </section>

          <section className="relative flex flex-col overflow-hidden rounded-2xl bg-[#0e352f] p-5 text-[#e9f4f0] shadow-[0_18px_40px_-30px_rgba(10,45,39,0.7)]">
            <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(340px_200px_at_100%_0%,rgba(35,146,122,0.2),transparent_70%)]" />
            <div className="relative flex min-h-0 flex-1 flex-col">
              <div className="text-[9.5px] font-semibold tracking-[0.18em] text-[#e9f4f0]/50 uppercase">
                Agent access
              </div>

              {device.mode === "direct" ? (
                <>
                  <h2 className="mt-2 text-[19px] font-semibold tracking-tight">Catalog only.</h2>
                  <p className="mt-1.5 text-[11.5px] leading-relaxed text-[#e9f4f0]/60">
                    The hub carries no traffic for this machine. An agent connects directly to the
                    address below.
                  </p>
                  <div className="mt-auto pt-4">
                    <DarkCode
                      value={directAddress}
                      onCopy={() => void copy(directAddress, "Device address copied")}
                    />
                  </div>
                </>
              ) : vncURL ? (
                <>
                  <h2 className="mt-2 text-[19px] font-semibold tracking-tight">
                    Hand this to an agent.
                  </h2>
                  <p className="mt-1.5 text-[11.5px] leading-relaxed text-[#e9f4f0]/60">
                    A standard VNC endpoint, scoped to this machine. Rotate the device token to
                    revoke it.
                  </p>
                  <div className="mt-3 space-y-2">
                    <DarkCode value={vncURL} onCopy={() => void copyAgent()} />
                    <DarkCode value={mcpURL} onCopy={() => void copy(mcpURL, "MCP endpoint copied")} />
                  </div>
                  <div className="mt-auto pt-4">
                    <button
                      type="button"
                      className="h-9 w-full rounded-lg bg-[#e5f3ed] text-[11.5px] font-medium text-[#0b3a33] transition-colors hover:bg-[#d8ece4]"
                      onClick={() => void copyAgent()}
                    >
                      {agentCopiesToken ? "Copy endpoint + token" : "Copy endpoint"}
                    </button>
                  </div>
                </>
              ) : (
                <>
                  <h2 className="mt-2 text-[19px] font-semibold tracking-tight">Browser only.</h2>
                  <p className="mt-1.5 text-[11.5px] leading-relaxed text-[#e9f4f0]/60">
                    This machine is reachable through the hub, but no raw VNC port is exposed for
                    agents.
                  </p>
                  <div className="mt-4">
                    <DarkCode value={mcpURL} onCopy={() => void copy(mcpURL, "MCP endpoint copied")} />
                  </div>
                  <div className="mt-auto pt-5">
                    <Button
                      variant="outline"
                      className="h-9 w-full border-white/15 bg-white/5 text-[11.5px] text-[#e9f4f0] hover:bg-white/10 hover:text-white"
                      onClick={onEdit}
                    >
                      Expose a port
                    </Button>
                  </div>
                </>
              )}
            </div>
          </section>
        </div>

        <StatStrip device={device} />

        <div className="mt-6 flex flex-wrap items-start justify-between gap-4 border-t border-border/60 pt-5">
          <p className="flex max-w-[460px] items-start gap-2 text-[10.5px] leading-relaxed text-muted-foreground/85">
            <Lock className="mt-0.5 size-3 shrink-0" />
            abacad never opens a connection of its own. A machine becomes online only after a real
            browser or agent session authenticates.
          </p>
          <div className="flex flex-wrap gap-1">
            <Button variant="ghost" size="sm" onClick={onEdit}>
              <Pencil className="size-3.5" />
              Edit
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              onClick={onRemove}
            >
              <Trash2 className="size-3.5" />
              Remove
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}
