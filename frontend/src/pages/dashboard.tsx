import { ArrowLeft, Copy, Monitor, Plus, Server } from "lucide-react"
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { BrandMark } from "@/components/brand-mark"
import { ConsoleRail } from "@/components/console-rail"
import { DeviceDialog } from "@/components/device-dialog"
import { LoginCard } from "@/components/login-card"
import { SkillPanel } from "@/components/skill-panel"
import { StatusDot } from "@/components/status-dot"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { VncFrame } from "@/components/vnc-frame"
import { ApiError, api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { deviceStatus, statusLabel } from "@/lib/device"
import type { Device, HubInfo } from "@/lib/types"

type AuthState = "loading" | "locked" | "in"

function Brand() {
  return (
    <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
      <BrandMark className="size-7 shrink-0" />
      <span className="text-[15px] font-semibold tracking-[-0.035em]">abacad</span>
    </Link>
  )
}

// The mark stands still in the chrome; while the hub is being reached it is the
// only thing on the page, so it is allowed to breathe.
function Splash({ label }: { label: string }) {
  return (
    <div className="grid min-h-screen place-items-center bg-background px-6">
      <div className="flex flex-col items-center gap-6">
        <BrandMark className="size-10 animate-pulse [animation-duration:2.4s]" />
        <p className="font-mono text-[10px] tracking-[0.16em] text-muted-foreground uppercase">
          {label}
        </p>
      </div>
    </div>
  )
}

function LoginScreen({
  busy,
  error,
  version,
  onSubmit,
}: {
  busy: boolean
  error: string
  version?: string
  onSubmit: (token: string) => void
}) {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="px-6 py-6 sm:px-10">
        <Brand />
      </header>
      <main className="flex flex-1 items-center justify-center px-5 pb-12">
        <div className="w-full max-w-[380px]">
          <p className="mb-3.5 flex items-center gap-2 font-mono text-[10px] tracking-[0.16em] text-muted-foreground uppercase">
            <span className="h-px w-5 bg-border" />
            self-hosted VNC hub
          </p>
          <h1 className="text-[26px] leading-[1.05] font-semibold tracking-[-0.04em] sm:text-[30px]">
            One quiet place.
          </h1>
          <p className="mt-2.5 text-[13px] leading-relaxed text-muted-foreground">
            Every screen on your LAN, behind one token.
          </p>
          <LoginCard busy={busy} error={error} onSubmit={onSubmit} />
        </div>
      </main>
      <footer className="mx-auto flex w-full max-w-6xl flex-col gap-1.5 px-6 py-5 font-mono text-[9.5px] tracking-[0.14em] text-muted-foreground uppercase sm:flex-row sm:items-center sm:justify-between sm:px-10">
        <span>{version ? `abacad v${version}` : "abacad"}</span>
        <span>no cloud · no relay · no account</span>
      </footer>
    </div>
  )
}

function Centered({ children }: { children: ReactNode }) {
  return (
    <div className="grid min-h-0 flex-1 place-items-center overflow-y-auto px-6 py-14">
      <div className="w-full max-w-md text-center">{children}</div>
    </div>
  )
}

function EmptyMain({ onAdd }: { onAdd: () => void }) {
  return (
    <Centered>
      <span className="mx-auto grid size-11 place-items-center rounded-xl border border-border bg-card text-muted-foreground shadow-card">
        <Server className="size-4.5" />
      </span>
      <h1 className="mt-5 text-[24px] leading-tight font-semibold tracking-[-0.035em]">
        No machines yet
      </h1>
      <p className="mx-auto mt-2 max-w-[38ch] text-[13px] leading-relaxed text-muted-foreground">
        Turn on Screen Sharing or a VNC server on a machine, then add its address. Nothing installs
        on the device.
      </p>
      <Button className="mt-6 h-9 px-3 text-[12.5px]" onClick={onAdd}>
        <Plus className="size-3.5" />
        Add your first device
      </Button>
      <p className="mx-auto mt-7 max-w-sm text-[11.5px] leading-relaxed text-muted-foreground">
        abacad does not scan your network. Add the machine&apos;s address by hand.
      </p>
    </Centered>
  )
}

// A catalog-only machine has no port through the hub, so there is no screen to
// show — just the address an agent or VNC client has to reach on its own.
function DirectMachine({ device, onEdit }: { device: Device; onEdit: () => void }) {
  const address = `${device.host}:${device.port}`
  const status = deviceStatus(device)
  return (
    <Centered>
      <span className="mx-auto grid size-11 place-items-center rounded-xl border border-border bg-card text-muted-foreground shadow-card">
        <Monitor className="size-4.5" />
      </span>
      <h1 className="mt-5 text-[24px] font-semibold tracking-[-0.035em]">{device.name}</h1>
      <p className="mt-2 flex items-center justify-center gap-2 text-[11.5px] text-muted-foreground">
        <StatusDot device={device} halo />
        {statusLabel(status)}
        <span className="text-faint">·</span>
        catalog only
      </p>
      <p className="mx-auto mt-3 max-w-[40ch] text-[13px] leading-relaxed text-muted-foreground">
        The hub carries no traffic for this machine. Point a VNC client or an agent at the address
        below.
      </p>
      <div className="mx-auto mt-6 flex w-full max-w-[320px] items-center gap-1 rounded-lg border border-input bg-card py-1 pr-1 pl-3 shadow-card">
        <span className="min-w-0 flex-1 truncate text-left font-mono text-[11.5px] text-foreground/80">
          {address}
        </span>
        <Button
          variant="ghost"
          size="icon-xs"
          className="text-muted-foreground/70 hover:text-foreground"
          aria-label="Copy address"
          onClick={() => void copy(address, "Device address copied")}
        >
          <Copy className="size-3.5" />
        </Button>
      </div>
      <Button variant="outline" className="mt-4 h-8 px-3 text-[12px]" onClick={onEdit}>
        Edit this machine
      </Button>
    </Centered>
  )
}

async function copy(value: string, message: string) {
  const copied = await copyText(value)
  if (copied) toast.success(message)
  else toast.error("Could not copy")
}

export default function Dashboard({ view = "devices" }: { view?: "devices" | "skill" }) {
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const [auth, setAuth] = useState<AuthState>("loading")
  const [ready, setReady] = useState(false)
  const [devices, setDevices] = useState<Device[]>([])
  const [info, setInfo] = useState<HubInfo | null>(null)
  const [hubError, setHubError] = useState(false)
  const [loginBusy, setLoginBusy] = useState(false)
  const [loginError, setLoginError] = useState("")
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Device | null>(null)
  const [removing, setRemoving] = useState<Device | null>(null)

  const load = useCallback(async (quiet = false) => {
    try {
      const deviceResponse = await api.devices()
      setDevices(deviceResponse.devices ?? [])
      setHubError(false)
    } catch (error) {
      if (error instanceof ApiError && error.status === 401) {
        setAuth("locked")
        setDevices([])
        return
      }
      setHubError(true)
      if (!quiet) toast.error(error instanceof Error ? error.message : "Could not load devices")
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    api
      .session()
      .then(async (session) => {
        if (cancelled) return
        if (session.authenticated) {
          setAuth("in")
          await load()
        } else {
          setAuth("locked")
        }
        if (!cancelled) setReady(true)
      })
      .catch(() => {
        if (!cancelled) {
          setAuth("locked")
          setReady(true)
        }
      })
    api
      .info()
      .then((value) => {
        if (!cancelled) setInfo(value)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [load])

  useEffect(() => {
    if (auth !== "in" || !ready) return
    const timer = window.setInterval(() => void load(true), 15000)
    const onVisibility = () => {
      if (!document.hidden) void load(true)
    }
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [auth, ready, load])

  const selected = useMemo(() => {
    if (id) return devices.find((device) => device.id === id) ?? null
    return devices.find((device) => device.online) ?? devices[0] ?? null
  }, [devices, id])

  async function handleLogin(token: string) {
    setLoginBusy(true)
    setLoginError("")
    try {
      await api.login(token)
      setAuth("in")
      setReady(false)
      await load()
      setReady(true)
      const next = searchParams.get("next")
      if (next && next.startsWith("/") && !next.startsWith("//")) {
        navigate(next, { replace: true })
      }
    } catch (error) {
      setLoginError(error instanceof Error ? error.message : "Could not sign in")
    } finally {
      setLoginBusy(false)
    }
  }

  function openAdd() {
    setEditing(null)
    setDialogOpen(true)
  }

  function openEdit(device: Device) {
    setEditing(device)
    setDialogOpen(true)
  }

  async function handleRemove(device: Device) {
    try {
      await api.removeDevice(device.id)
      toast.success(`${device.name} removed`)
      if (selected?.id === device.id) navigate("/", { replace: true })
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not remove the device")
    }
  }

  async function handleSaved(saved: Device) {
    const adding = editing === null
    toast.success(adding ? "Device added" : "Device updated")
    setEditing(null)
    await load()
    if (adding) {
      navigate(`/devices/${encodeURIComponent(saved.id)}`, { replace: true })
    }
  }

  if (auth === "loading" || (auth === "in" && !ready)) {
    return <Splash label={auth === "loading" ? "Connecting…" : "Loading devices…"} />
  }

  if (auth === "locked") {
    return (
      <LoginScreen
        busy={loginBusy}
        error={loginError}
        version={info?.version}
        onSubmit={handleLogin}
      />
    )
  }

  let main: ReactNode
  if (view === "skill") {
    main = <SkillPanel devices={devices} />
  } else if (selected) {
    main =
      selected.mode === "direct" ? (
        <DirectMachine
          key={selected.id}
          device={selected}
          onEdit={() => openEdit(selected)}
        />
      ) : (
        <VncFrame key={selected.id} className="min-h-0 flex-1" device={selected} />
      )
  } else if (devices.length === 0) {
    if (hubError) {
      main = (
        <Centered>
          <h1 className="text-[24px] font-semibold tracking-[-0.035em]">Can’t reach the hub</h1>
          <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">
            The page is signed in, but the device list did not load. The hub may be restarting.
          </p>
          <Button className="mt-6" variant="outline" onClick={() => void load()}>
            Try again
          </Button>
        </Centered>
      )
    } else {
      main = <EmptyMain onAdd={() => openAdd()} />
    }
  } else {
    main = (
      <Centered>
        <h1 className="text-[24px] font-semibold tracking-[-0.035em]">Machine not found</h1>
        <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">
          It may have been removed, or the link is stale.
        </p>
        <Button asChild variant="outline" className="mt-6">
          <Link to="/">
            <ArrowLeft className="size-3.5" />
            Back to devices
          </Link>
        </Button>
      </Centered>
    )
  }

  return (
    <div className="flex h-dvh min-w-0 flex-col lg:grid lg:h-screen lg:grid-cols-[288px_minmax(0,1fr)] lg:overflow-hidden">
      <ConsoleRail
        devices={devices}
        selectedId={view === "skill" ? undefined : selected?.id}
        skillActive={view === "skill"}
        onAdd={() => openAdd()}
        onEdit={(device) => openEdit(device)}
        onRemove={(device) => setRemoving(device)}
      />
      <main className="flex min-h-0 min-w-0 flex-1 flex-col lg:h-screen lg:overflow-hidden">
        {main}
      </main>

      <DeviceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        device={editing}
        onSaved={(saved) => void handleSaved(saved)}
      />

      {/* The one destructive action in the hub. Radix keeps it modal until the
          human picks a side; the machine's own VNC server is never touched. */}
      <AlertDialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove {removing?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              The hub forgets the address, the sign-in and the agent token. The VNC server on the
              machine keeps running.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel className="h-9 px-3 text-[12.5px]">Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="h-9 bg-destructive px-3.5 text-[12.5px] text-white hover:bg-destructive/90"
              onClick={() => {
                const device = removing
                if (device) void handleRemove(device)
              }}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
