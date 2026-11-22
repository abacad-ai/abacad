import { useCallback, useEffect, useState } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { AppHeader, type HubStatus } from "@/components/app-header"
import { DiscoveredList } from "@/components/discovered-list"
import { DeviceCard } from "@/components/device-card"
import { DeviceDialog, type DevicePrefill } from "@/components/device-dialog"
import { LoginCard } from "@/components/login-card"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ApiError, api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import type { Device, DiscoveredDevice, HubInfo } from "@/lib/types"

type AuthState = "loading" | "locked" | "in"

export default function Dashboard() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [auth, setAuth] = useState<AuthState>("loading")
  const [devices, setDevices] = useState<Device[]>([])
  const [discovered, setDiscovered] = useState<DiscoveredDevice[]>([])
  const [info, setInfo] = useState<HubInfo | null>(null)
  const [hubError, setHubError] = useState(false)
  const [loginBusy, setLoginBusy] = useState(false)
  const [loginError, setLoginError] = useState("")
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<Device | null>(null)
  const [prefill, setPrefill] = useState<DevicePrefill | undefined>()

  const load = useCallback(async (quiet = false) => {
    try {
      const [deviceResponse, discoveredResponse] = await Promise.all([
        api.devices(),
        api.discovered().catch(() => ({ devices: [] })),
      ])
      setDevices(deviceResponse.devices ?? [])
      setDiscovered(discoveredResponse.devices ?? [])
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
      })
      .catch(() => {
        if (!cancelled) setAuth("locked")
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
    if (auth !== "in") return
    const timer = window.setInterval(() => void load(true), 15000)
    const onVisibility = () => {
      if (!document.hidden) void load(true)
    }
    document.addEventListener("visibilitychange", onVisibility)
    return () => {
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", onVisibility)
    }
  }, [auth, load])

  async function handleLogin(token: string) {
    setLoginBusy(true)
    setLoginError("")
    try {
      await api.login(token)
      setAuth("in")
      await load()
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
    setDiscovered([])
    navigate("/", { replace: true })
  }

  function openAdd(nextPrefill?: DevicePrefill) {
    setEditing(null)
    setPrefill(nextPrefill)
    setDialogOpen(true)
  }

  function openEdit(device: Device) {
    setEditing(device)
    setPrefill(undefined)
    setDialogOpen(true)
  }

  async function handleRemove(device: Device) {
    if (!window.confirm(`Remove ${device.name}? The VNC server itself is not touched.`)) return
    try {
      await api.removeDevice(device.id)
      toast.success(`${device.name} removed`)
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not remove the device")
    }
  }

  async function handleProbe(id: string) {
    try {
      await api.probe(id)
      await load()
      toast.success("Device checked")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Could not check the device")
    }
  }

  async function handleCopyVNC(device: Device) {
    if (!device.vnc_port) return
    const url = `vnc://${window.location.hostname || "127.0.0.1"}:${device.vnc_port}`
    if (device.mode === "l4") {
      const copied = await copyText(url)
      if (copied) toast.success("VNC address copied — use the device's VNC password")
      else toast.error("Could not copy")
      return
    }
    const copied = await copyText(`${url}\npassword: ${device.token ?? ""}`)
    if (copied) toast.success("VNC address and token copied")
    else toast.error("Could not copy")
  }

  async function handleCopyToken(device: Device) {
    if (!device.token) return
    const copied = await copyText(device.token)
    if (copied) toast.success("Token copied")
    else toast.error("Could not copy")
  }

  const known = new Set(devices.map((device) => `${device.host}:${device.port}`))
  const discoveredEntries = discovered.filter((entry) => !known.has(`${entry.host}:${entry.port}`))

  const status: HubStatus =
    auth === "in"
      ? hubError
        ? { label: "hub offline", tone: "offline" }
        : { label: "hub online", tone: "online" }
      : auth === "loading"
        ? { label: "connecting", tone: "idle" }
        : { label: "locked", tone: "idle" }

  return (
    <div className="flex min-h-screen flex-col">
      <AppHeader
        signedIn={auth === "in"}
        status={status}
        onAdd={() => openAdd()}
        onLogout={handleLogout}
      />

      <main className="mx-auto w-full max-w-6xl flex-1 px-5 pb-20">
        <section className="pt-8 pb-10 sm:pt-14 sm:pb-14">
          <p className="text-[11px] font-medium tracking-[0.24em] text-primary uppercase">
            LAN VNC hub
          </p>
          <h1 className="mt-3 text-5xl leading-[0.95] tracking-[-0.03em] sm:text-6xl">
            Your machines,
            <br />
            <em className="font-display font-normal text-primary">one quiet place.</em>
          </h1>
          <p className="mt-5 max-w-xl text-sm leading-relaxed text-muted-foreground">
            A tiny, self-hosted hub for the VNC servers already on your network. Open a browser
            tab — or hand an agent a standard VNC endpoint.
          </p>
        </section>

        {auth === "loading" ? (
          <p className="py-16 text-center text-sm text-muted-foreground">Connecting…</p>
        ) : null}

        {auth === "locked" ? (
          <LoginCard busy={loginBusy} error={loginError} onSubmit={handleLogin} />
        ) : null}

        {auth === "in" ? (
          <>
            <section>
              <div className="flex items-end justify-between gap-3">
                <h2 className="text-lg font-medium tracking-tight">Devices</h2>
                <span className="text-sm text-muted-foreground">
                  {devices.length ? `${devices.length} device${devices.length === 1 ? "" : "s"}` : ""}
                </span>
              </div>

              {devices.length === 0 ? (
                <Card className="mt-4 items-center gap-3 border-dashed bg-card/60 px-6 py-12 text-center">
                  <div className="grid size-12 place-items-center rounded-full bg-muted text-xl text-muted-foreground">
                    ◌
                  </div>
                  <h3 className="text-base font-medium">No devices yet</h3>
                  <p className="max-w-md text-sm text-muted-foreground">
                    Turn on Screen Sharing or a VNC server on a machine, then add its address.
                    Nothing installs on the device.
                  </p>
                  <Button className="mt-2" onClick={() => openAdd()}>
                    Add your first device
                  </Button>
                </Card>
              ) : (
                <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                  {devices.map((device) => (
                    <DeviceCard
                      key={device.id}
                      device={device}
                      onEdit={openEdit}
                      onProbe={handleProbe}
                      onRemove={handleRemove}
                      onCopyVNC={handleCopyVNC}
                      onCopyToken={handleCopyToken}
                    />
                  ))}
                </div>
              )}
            </section>

            {discoveredEntries.length > 0 ? (
              <section className="mt-10">
                <div className="flex items-end justify-between gap-3">
                  <h2 className="text-lg font-medium tracking-tight">Discovered on the LAN</h2>
                  <span className="text-sm text-muted-foreground">
                    mDNS · <code className="font-mono text-xs">_rfb._tcp</code>
                  </span>
                </div>
                <DiscoveredList
                  entries={discoveredEntries}
                  onAdd={(entry) =>
                    openAdd({ name: entry.name, host: entry.host, port: entry.port })
                  }
                />
              </section>
            ) : null}
          </>
        ) : null}
      </main>

      <footer className="mx-auto flex w-full max-w-6xl items-center justify-between px-5 py-6 text-xs text-muted-foreground">
        <span>
          abacad {info?.version ? `v${info.version}` : ""}
        </span>
        <span>LAN only · no cloud · no relay</span>
      </footer>

      <DeviceDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        device={editing}
        prefill={prefill}
        onSaved={() => {
          void load()
          toast.success(editing ? "Device updated" : "Device added")
        }}
      />
    </div>
  )
}
