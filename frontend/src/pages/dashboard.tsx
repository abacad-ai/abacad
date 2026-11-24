import { ArrowLeft, Plus, Server } from "lucide-react"
import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { ConsoleRail } from "@/components/console-rail"
import { DeviceDetail } from "@/components/device-detail"
import { DeviceDialog } from "@/components/device-dialog"
import { LoginCard } from "@/components/login-card"
import { Button } from "@/components/ui/button"
import { ApiError, api } from "@/lib/api"
import type { Device, HubInfo } from "@/lib/types"

type AuthState = "loading" | "locked" | "in"

function Brand() {
  return (
    <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
      <span className="grid size-8 place-items-center rounded-[10px] bg-primary text-[17px] font-semibold text-primary-foreground shadow-sm">
        a
      </span>
      <span className="text-[15px] font-semibold tracking-tight">abacad</span>
    </Link>
  )
}

function Splash({ label }: { label: string }) {
  return (
    <div className="grid min-h-screen place-items-center bg-background px-6">
      <div className="flex flex-col items-center gap-4">
        <span className="grid size-11 animate-pulse place-items-center rounded-xl bg-primary text-lg font-semibold text-primary-foreground shadow-sm">
          a
        </span>
        <p className="text-xs text-muted-foreground">{label}</p>
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
      <main className="flex flex-1 items-center justify-center px-5 pb-16">
        <div className="w-full max-w-[400px]">
          <p className="mb-7 text-center font-display text-[26px] text-primary italic">
            one quiet place.
          </p>
          <LoginCard busy={busy} error={error} onSubmit={onSubmit} />
        </div>
      </main>
      <footer className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-5 text-[11px] text-muted-foreground/70 sm:px-10">
        <span>{version ? `abacad v${version}` : "abacad"}</span>
        <span>LAN only · no cloud · no relay</span>
      </footer>
    </div>
  )
}

function EmptyMain({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col">
      <header className="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between gap-3 border-b border-border bg-background/85 px-4 backdrop-blur-md lg:px-8">
        <span className="text-[11.5px] text-muted-foreground">Devices</span>
        <Button size="lg" className="h-9 px-3.5 text-[12.5px]" onClick={onAdd}>
          <Plus />
          Add device
        </Button>
      </header>

      <div className="grid flex-1 place-items-center px-5 py-16">
        <div className="w-full max-w-md text-center">
          <span className="mx-auto grid size-12 place-items-center rounded-2xl bg-accent text-accent-foreground">
            <Server className="size-5" />
          </span>
          <h1 className="mt-4 text-[26px] font-semibold tracking-tight">No machines yet</h1>
          <p className="mx-auto mt-2 max-w-sm text-[13px] leading-relaxed text-muted-foreground">
            Turn on Screen Sharing or a VNC server on a machine, then add its address. Nothing
            installs on the device.
          </p>
          <Button className="mt-5 h-9 px-3.5 text-[12.5px]" onClick={onAdd}>
            <Plus />
            Add your first device
          </Button>

          <p className="mx-auto mt-6 max-w-sm text-[11px] leading-relaxed text-muted-foreground/80">
            abacad does not scan your network. Add the machine&apos;s address by hand.
          </p>
        </div>
      </div>
      <footer className="mx-auto flex w-full max-w-6xl items-center justify-between px-6 py-5 text-[11px] text-muted-foreground/70 lg:px-8">
        <span>LAN only</span>
        <span>no cloud · no relay</span>
      </footer>
    </div>
  )
}

function MissingMain({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col">
      <header className="flex h-16 shrink-0 items-center border-b border-border px-4 lg:px-8">
        <span className="text-[11.5px] text-muted-foreground">Devices</span>
      </header>
      <div className="grid flex-1 place-items-center px-5 py-16 text-center">
        <div className="max-w-sm">{children}</div>
      </div>
    </div>
  )
}

export default function Dashboard() {
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

  async function handleLogout() {
    await api.logout().catch(() => {})
    setAuth("locked")
    setDevices([])
    setReady(true)
    navigate("/", { replace: true })
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
    if (!window.confirm(`Remove ${device.name}? The VNC server itself is not touched.`)) return
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
    toast.success(adding ? "Device added — open it once to verify." : "Device updated")
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

  const hubOnline = auth === "in" && !hubError

  let main: ReactNode
  if (selected) {
    main = (
      <DeviceDetail
        device={selected}
        onEdit={() => openEdit(selected)}
        onRemove={() => void handleRemove(selected)}
      />
    )
  } else if (devices.length === 0) {
    if (hubError) {
      main = (
        <MissingMain>
          <h1 className="text-[22px] font-semibold tracking-tight">Can’t reach the hub</h1>
          <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">
            The page is signed in, but the device list did not load. The hub may be restarting.
          </p>
          <Button className="mt-5" variant="outline" onClick={() => void load()}>
            Try again
          </Button>
        </MissingMain>
      )
    } else {
      main = (
        <EmptyMain onAdd={() => openAdd()} />
      )
    }
  } else {
    main = (
      <MissingMain>
        <h1 className="text-[22px] font-semibold tracking-tight">Machine not found</h1>
        <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">
          It may have been removed, or the link is stale.
        </p>
        <Button asChild variant="outline" className="mt-5">
          <Link to="/">
            <ArrowLeft className="size-3.5" />
            Back to devices
          </Link>
        </Button>
      </MissingMain>
    )
  }

  return (
    <div className="flex min-h-screen min-w-0 flex-col lg:grid lg:h-screen lg:grid-cols-[296px_minmax(0,1fr)] lg:overflow-hidden">
      <ConsoleRail
        devices={devices}
        selectedId={selected?.id}
        hubOnline={hubOnline}
        version={info?.version}
        onAdd={() => openAdd()}
        onLogout={() => void handleLogout()}
      />
      <main className="flex min-h-0 min-w-0 flex-1 flex-col lg:h-screen lg:overflow-y-auto">{main}</main>

      <DeviceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        device={editing}
        onSaved={(saved) => void handleSaved(saved)}
      />
    </div>
  )
}
