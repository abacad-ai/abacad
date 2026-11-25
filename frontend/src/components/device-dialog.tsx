import { Lock } from "lucide-react"
import { useEffect, useState, type FormEvent } from "react"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { api } from "@/lib/api"
import type { Device, DeviceMode, DevicePayload, SignIn } from "@/lib/types"

const platformOptions = [
  { value: "unknown", label: "Unknown" },
  { value: "macos", label: "macOS" },
  { value: "linux", label: "Linux" },
  { value: "windows", label: "Windows" },
  { value: "android", label: "Android" },
]

const modeOptions: { value: DeviceMode; label: string }[] = [
  { value: "gateway", label: "Gateway (recommended)" },
  { value: "l4", label: "Raw TCP (L4)" },
  { value: "direct", label: "Catalog only" },
]

export function DeviceDialog({
  open,
  onOpenChange,
  device,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  device: Device | null
  onSaved: (device: Device) => void
}) {
  const editing = device !== null
  const [name, setName] = useState("")
  const [host, setHost] = useState("")
  const [port, setPort] = useState("5900")
  const [signIn, setSignIn] = useState<SignIn>("vnc")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [platform, setPlatform] = useState("unknown")
  const [mode, setMode] = useState<DeviceMode>("gateway")
  const [expose, setExpose] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!open) return
    setName(device?.name ?? "")
    setHost(device?.host ?? "")
    setPort(String(device?.port ?? 5900))
    setSignIn(
      device?.auth ?? (device?.platform === "macos" ? "ard" : "vnc"),
    )
    setUsername(device?.username ?? "")
    setPassword("")
    setPlatform(device?.platform ?? "unknown")
    setMode(device?.mode ?? "gateway")
    setExpose(Boolean(device?.vnc_port))
    setBusy(false)
    setError("")
  }, [open, device])

  const authChanged = editing && device?.auth !== signIn
  const passwordRequired = signIn === "ard" && (!editing || authChanged)
  const passwordPlaceholder = editing && !authChanged
    ? "leave blank to keep the current password"
    : signIn === "ard"
      ? "the password you use to log in to the Mac"
      : "the password you set on the device"

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError("")

    if (signIn === "ard" && username.trim() === "") {
      setError("macOS username is required for macOS account sign-in.")
      return
    }
    if (passwordRequired && password === "") {
      setError(signIn === "ard" ? "Enter the macOS account password." : "Enter the device password.")
      return
    }
    if (editing && authChanged && password === "") {
      setError("Enter the password for the new sign-in method.")
      return
    }

    const payload: DevicePayload = {
      name: name.trim(),
      host: host.trim(),
      port: Number(port) || 5900,
      auth: signIn,
      username: username.trim(),
      platform: platform === "unknown" ? "" : platform,
      mode,
      expose,
    }
    if (password !== "") payload.password = password

    setBusy(true)
    try {
      const response =
        editing && device
          ? await api.updateDevice(device.id, payload)
          : await api.addDevice(payload)
      onSaved(response.device)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the device.")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{editing ? `Edit ${device?.name || "device"}` : "Add a device"}</DialogTitle>
          <DialogDescription>
            No software is installed on the device. abacad only needs its address and sign-in.
          </DialogDescription>
        </DialogHeader>

        <form className="space-y-4" onSubmit={submit} autoComplete="off">
          <div className="space-y-2">
            <Label htmlFor="device-name">Name</Label>
            <Input
              id="device-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Mac mini"
              required
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-[1fr_7rem]">
            <div className="space-y-2">
              <Label htmlFor="device-host">Host</Label>
              <Input
                id="device-host"
                value={host}
                onChange={(event) => setHost(event.target.value)}
                placeholder="192.168.1.20"
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="device-port">Port</Label>
              <Input
                id="device-port"
                type="number"
                min={1}
                max={65535}
                value={port}
                onChange={(event) => setPort(event.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label>Sign-in</Label>
            <Select value={signIn} onValueChange={(value) => setSignIn(value as SignIn)}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="vnc">VNC password</SelectItem>
                <SelectItem value="ard">macOS account (recommended for Mac)</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {signIn === "ard" ? (
            <div className="space-y-2">
              <Label htmlFor="device-username">macOS username</Label>
              <Input
                id="device-username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                placeholder="li"
                autoComplete="off"
                required
              />
              <p className="text-[11.5px] text-muted-foreground">The short name, as printed by <code className="font-mono text-[11px] text-foreground/75">whoami</code>.</p>
            </div>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor="device-password">
              {signIn === "ard" ? "macOS account password" : "VNC password"}
            </Label>
            <Input
              id="device-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder={passwordPlaceholder}
              autoComplete="new-password"
              required={passwordRequired}
            />
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-2">
              <Label>Platform</Label>
              <Select
                value={platform}
                onValueChange={(value) => {
                  setPlatform(value)
                  if (!editing && value === "macos") setSignIn("ard")
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {platformOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Mode</Label>
              <Select value={mode} onValueChange={(value) => setMode(value as DeviceMode)}>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {modeOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <label className="flex items-start gap-2.5 text-[12.5px] text-foreground/85">
            <Checkbox
              checked={expose}
              onCheckedChange={(value) => setExpose(value === true)}
              className="mt-0.5"
            />
            <span>Also open a raw VNC port for agents</span>
          </label>

          <p className="flex items-start gap-2 text-[11.5px] leading-relaxed text-muted-foreground">
            <Lock className="mt-0.5 size-3.5 shrink-0 text-faint" />
            <span>
              abacad never probes the device. It shows online only after a real browser or agent
              session authenticates — open it once to confirm the address and password.
            </span>
          </p>

          <p className="rounded-lg border border-border bg-muted/60 px-3 py-2.5 text-[11.5px] leading-relaxed text-muted-foreground">
            Gateway keeps the device password in the hub and gives agents a revocable token.
            On macOS, <strong className="text-foreground">macOS account</strong> signs in as the
            logged-in user; the legacy VNC password has no user identity and can show only the
            login window.
          </p>

          {error ? (
            <p role="alert" className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-[12px] text-destructive">
              {error}
            </p>
          ) : null}

          <DialogFooter className="pt-4">
            <Button type="button" variant="ghost" className="h-9 px-3 text-[12.5px]" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" className="h-9 px-3.5 text-[12.5px]" disabled={busy}>
              {busy ? "Saving…" : editing ? "Save changes" : "Add device"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
