import {
  ArrowLeft,
  Clipboard,
  Command,
  Keyboard,
  Loader2,
  Maximize2,
  RotateCcw,
} from "lucide-react"
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ApiError, api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import type { Device } from "@/lib/types"
import { useVncSession, type VncStatus } from "@/lib/vnc"

const TAB_KEYSYM = 0xff09
const ESCAPE_KEYSYM = 0xff1b

function sessionUrl(id: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  return `${protocol}//${window.location.host}/ws/vnc/${encodeURIComponent(id)}`
}

export default function Viewer() {
  const { id = "" } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const shellRef = useRef<HTMLDivElement>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)

  const [device, setDevice] = useState<Device | null>(null)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState("")
  const [panel, setPanel] = useState<"keys" | "clipboard" | null>(null)
  const [draft, setDraft] = useState("")
  const [hintVisible, setHintVisible] = useState(true)

  const url = useMemo(
    () => (device && device.mode !== "direct" ? sessionUrl(id) : ""),
    [device, id],
  )
  const {
    containerRef,
    status,
    error: sessionError,
    clipboard,
    modifiers,
    credentialsRequest,
    sendCredentials,
    reconnect,
    focus,
    paste,
    toggleModifier,
    sendKey,
    sendCtrlAltDel,
  } = useVncSession(url)

  useEffect(() => {
    let cancelled = false
    // Switching devices reuses this component, so the previous result has to be
    // cleared before the new fetch starts.
    // oxlint-disable-next-line react/set-state-in-effect
    setLoading(true)
    setLoadError("")
    api
      .devices()
      .then((response) => {
        if (cancelled) return
        const found = response.devices.find((candidate) => candidate.id === id) ?? null
        if (!found) {
          setLoadError("Device not found.")
          setLoading(false)
          return
        }
        if (found.mode === "direct") {
          setLoadError(
            `This device is catalog-only. Connect your VNC client directly to ${found.host}:${found.port}.`,
          )
          setLoading(false)
          return
        }
        setDevice(found)
        setLoading(false)
      })
      .catch((caught) => {
        if (cancelled) return
        if (caught instanceof ApiError && caught.status === 401) {
          navigate(`/?next=${encodeURIComponent(`/vnc/${id}`)}`, { replace: true })
          return
        }
        setLoadError(caught instanceof Error ? caught.message : "Could not load the device.")
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id, navigate])

  // Dismiss an open panel the same way a popover would.
  useEffect(() => {
    if (!panel) return
    const onPointerDown = (event: PointerEvent) => {
      if (!toolbarRef.current?.contains(event.target as Node)) setPanel(null)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setPanel(null)
    }
    document.addEventListener("pointerdown", onPointerDown)
    document.addEventListener("keydown", onKeyDown)
    return () => {
      document.removeEventListener("pointerdown", onPointerDown)
      document.removeEventListener("keydown", onKeyDown)
    }
  }, [panel])

  function togglePanel(next: "keys" | "clipboard") {
    if (next === "clipboard" && panel !== "clipboard") setDraft(clipboard)
    setPanel((current) => (current === next ? null : next))
  }

  function startOver() {
    setHintVisible(true)
    reconnect()
  }

  function fullscreen() {
    void shellRef.current?.requestFullscreen?.().catch(() => {})
  }

  const connected = status === "connected"

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center bg-neutral-950 text-sm text-neutral-300">
        Connecting…
      </div>
    )
  }

  if (loadError) {
    return (
      <div className="grid min-h-screen place-items-center p-6">
        <Card className="max-w-md items-center gap-3 p-6 text-center">
          <p className="text-base font-medium">Can’t open this device</p>
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <Button asChild variant="outline" className="mt-2">
            <Link to="/">
              <ArrowLeft />
              Back to devices
            </Link>
          </Button>
        </Card>
      </div>
    )
  }

  return (
    <div ref={shellRef} className="flex h-screen flex-col bg-neutral-950">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-white/10 bg-neutral-950 px-2 text-neutral-200">
        <Button asChild size="sm" variant="ghost" className="text-neutral-300 hover:bg-white/10 hover:text-white">
          <Link to={`/devices/${encodeURIComponent(id)}`}>Hub</Link>
        </Button>
        <span className="min-w-0 truncate font-mono text-xs text-neutral-400">
          {device?.name || id}
        </span>
        <StatusPill status={status} />
        <div ref={toolbarRef} className="relative ml-auto flex items-center gap-1">
          <ToolbarButton disabled={!connected} onClick={focus}>
            <Keyboard />
            <span className="hidden sm:inline">Keyboard</span>
          </ToolbarButton>
          <ToolbarButton
            disabled={!connected}
            expanded={panel === "keys"}
            onClick={() => togglePanel("keys")}
          >
            <Command />
            <span className="hidden sm:inline">Keys</span>
          </ToolbarButton>
          <ToolbarButton
            disabled={!connected}
            expanded={panel === "clipboard"}
            onClick={() => togglePanel("clipboard")}
          >
            <Clipboard />
            <span className="hidden sm:inline">Clipboard</span>
          </ToolbarButton>
          <ToolbarButton onClick={fullscreen}>
            <Maximize2 />
            <span className="hidden sm:inline">Fullscreen</span>
          </ToolbarButton>
          <ToolbarButton onClick={startOver}>
            <RotateCcw />
            <span className="hidden sm:inline">Reconnect</span>
          </ToolbarButton>

          {panel === "keys" ? (
            <div className="absolute top-full right-0 z-20 mt-2 w-max rounded-xl bg-neutral-900 p-3 text-neutral-100 shadow-xl ring-1 ring-white/10">
              <p className="mb-2 text-xs text-neutral-400">Hold a modifier, then click the screen.</p>
              <div className="grid grid-cols-3 gap-1">
                <PanelButton pressed={modifiers.ctrl} onClick={() => toggleModifier("ctrl")}>
                  Ctrl
                </PanelButton>
                <PanelButton pressed={modifiers.alt} onClick={() => toggleModifier("alt")}>
                  Alt
                </PanelButton>
                <PanelButton pressed={modifiers.meta} onClick={() => toggleModifier("meta")}>
                  Win
                </PanelButton>
                <PanelButton onClick={() => sendKey(TAB_KEYSYM, "Tab")}>Tab</PanelButton>
                <PanelButton onClick={() => sendKey(ESCAPE_KEYSYM, "Escape")}>Esc</PanelButton>
                <PanelButton onClick={sendCtrlAltDel}>Ctrl+Alt+Del</PanelButton>
              </div>
            </div>
          ) : null}

          {panel === "clipboard" ? (
            <div className="absolute top-full right-0 z-20 mt-2 w-[min(92vw,26rem)] rounded-xl bg-neutral-900 p-3 text-neutral-100 shadow-xl ring-1 ring-white/10">
              <p className="mb-2 text-xs text-neutral-400">
                One clipboard for both machines. Send text to the remote, or copy what it sent.
              </p>
              <textarea
                value={draft}
                onChange={(event) => setDraft(event.target.value)}
                rows={6}
                spellCheck={false}
                placeholder="Text to send to the remote machine"
                className="w-full resize-y rounded-lg border border-white/10 bg-neutral-950 p-2 font-mono text-xs text-neutral-100 outline-none focus:border-white/30"
              />
              <div className="mt-2 flex items-center justify-between gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-neutral-300 hover:bg-white/10 hover:text-white"
                  disabled={!clipboard}
                  onClick={() => void copyText(clipboard)}
                >
                  Copy from remote
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="border-white/15 bg-white/5 text-neutral-100 hover:bg-white/10 hover:text-white"
                  disabled={!draft}
                  onClick={() => paste(draft)}
                >
                  Send
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </header>

      <div className="relative min-h-0 flex-1">
        <div
          ref={containerRef}
          className="absolute inset-0"
          onPointerDown={() => setHintVisible(false)}
        />

        {connected && hintVisible ? (
          <div className="pointer-events-none absolute inset-x-0 bottom-5 mx-auto w-fit max-w-[90%] rounded-full border border-white/10 bg-black/70 px-4 py-2 text-center text-xs text-white/80 backdrop-blur">
            Click the screen, then type · macOS lock screen: press Space or click your user
          </div>
        ) : null}

        {status === "connecting" && !credentialsRequest ? (
          <Overlay>
            <Loader2 className="size-5 animate-spin text-neutral-300" />
            <p className="text-sm text-neutral-300">Connecting to {device?.name || id}…</p>
          </Overlay>
        ) : null}

        {credentialsRequest ? (
          <Overlay>
            <CredentialsPrompt types={credentialsRequest} onSubmit={sendCredentials} />
          </Overlay>
        ) : null}

        {status === "reconnecting" ? (
          <Overlay>
            <Loader2 className="size-5 animate-spin text-amber-300" />
            <p className="text-sm text-neutral-200">Connection lost — reconnecting…</p>
            {sessionError ? (
              <p className="max-w-md text-center text-xs text-neutral-400">{sessionError}</p>
            ) : null}
            <OutlineButton onClick={startOver}>Retry now</OutlineButton>
          </Overlay>
        ) : null}

        {status === "failed" ? (
          <Overlay>
            <p className="max-w-md text-center text-sm text-neutral-200">{sessionError}</p>
            <div className="flex items-center gap-2">
              <OutlineButton onClick={startOver}>
                <RotateCcw />
                Retry
              </OutlineButton>
              <Button asChild size="sm" variant="ghost" className="text-neutral-300 hover:bg-white/10 hover:text-white">
                <Link to="/">Back to devices</Link>
              </Button>
            </div>
          </Overlay>
        ) : null}
      </div>
    </div>
  )
}

function ToolbarButton({
  disabled,
  expanded,
  onClick,
  children,
}: {
  disabled?: boolean
  expanded?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Button
      size="sm"
      variant="ghost"
      disabled={disabled}
      aria-expanded={expanded}
      className="text-neutral-300 hover:bg-white/10 hover:text-white"
      onClick={onClick}
    >
      {children}
    </Button>
  )
}

function OutlineButton({ onClick, children }: { onClick: () => void; children: ReactNode }) {
  return (
    <Button
      size="sm"
      variant="outline"
      className="border-white/15 bg-white/5 text-neutral-100 hover:bg-white/10 hover:text-white"
      onClick={onClick}
    >
      {children}
    </Button>
  )
}

function CredentialsPrompt({
  types,
  onSubmit,
}: {
  types: string[]
  onSubmit: (credentials: { username?: string; password: string }) => void
}) {
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const wantsUser = types.includes("username")

  return (
    <form
      className="w-[min(90vw,22rem)] rounded-xl bg-neutral-900 p-4 text-neutral-100 shadow-xl ring-1 ring-white/10"
      onSubmit={(event) => {
        event.preventDefault()
        onSubmit({ username: wantsUser ? username : undefined, password })
      }}
    >
      <p className="text-sm font-medium">This device wants a sign-in</p>
      <p className="mt-1 text-xs text-neutral-400">
        This device is bridged straight through, so it keeps its own RFB password.
      </p>
      {wantsUser ? (
        <input
          value={username}
          onChange={(event) => setUsername(event.target.value)}
          autoComplete="username"
          placeholder="Username"
          className="mt-3 w-full rounded-lg border border-white/10 bg-neutral-950 px-2 py-1.5 text-sm outline-none focus:border-white/30"
        />
      ) : null}
      <input
        value={password}
        onChange={(event) => setPassword(event.target.value)}
        type="password"
        autoComplete="current-password"
        autoFocus
        placeholder="Password"
        className="mt-2 w-full rounded-lg border border-white/10 bg-neutral-950 px-2 py-1.5 text-sm outline-none focus:border-white/30"
      />
      <Button type="submit" size="sm" className="mt-3 w-full" disabled={!password}>
        Unlock
      </Button>
    </form>
  )
}

function StatusPill({ status }: { status: VncStatus }) {
  const tone =
    status === "connected"
      ? "bg-emerald-400"
      : status === "failed"
        ? "bg-red-400"
        : "bg-amber-300 animate-pulse"
  const label =
    status === "connected"
      ? "Connected"
      : status === "failed"
        ? "Disconnected"
        : status === "reconnecting"
          ? "Reconnecting"
          : "Connecting"
  return (
    <span className="hidden items-center gap-1.5 rounded-full border border-white/10 px-2 py-0.5 text-[0.7rem] text-neutral-400 sm:inline-flex">
      <span className={`size-1.5 rounded-full ${tone}`} />
      {label}
    </span>
  )
}

function PanelButton({
  pressed,
  onClick,
  children,
}: {
  pressed?: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <Button
      size="sm"
      variant="ghost"
      aria-pressed={pressed}
      className={
        pressed
          ? "bg-white/15 text-white hover:bg-white/20 hover:text-white"
          : "text-neutral-300 hover:bg-white/10 hover:text-white"
      }
      onClick={onClick}
    >
      {children}
    </Button>
  )
}

function Overlay({ children }: { children: ReactNode }) {
  return (
    <div className="absolute inset-0 z-10 grid place-items-center bg-neutral-950/70 backdrop-blur-sm">
      <div className="flex flex-col items-center gap-3">{children}</div>
    </div>
  )
}
