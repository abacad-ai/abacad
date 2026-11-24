import { Clipboard, Command, Keyboard, Loader2, Maximize2, RotateCcw } from "lucide-react"
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"

import { Button } from "@/components/ui/button"
import { copyText } from "@/lib/clipboard"
import type { Device } from "@/lib/types"
import { cn } from "@/lib/utils"
import { useVncSession, type VncStatus } from "@/lib/vnc"

const TAB_KEYSYM = 0xff09
const ESCAPE_KEYSYM = 0xff1b

function sessionUrl(id: string) {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  return `${protocol}//${window.location.host}/ws/vnc/${encodeURIComponent(id)}`
}

// The live screen, its toolbar and its overlays. The console mounts this right
// next to the device list; the standalone /vnc page mounts the same stage with
// a way back.
export function VncStage({ device, leading }: { device: Device; leading?: ReactNode }) {
  const shellRef = useRef<HTMLDivElement>(null)
  const toolbarRef = useRef<HTMLDivElement>(null)

  const [panel, setPanel] = useState<"keys" | "clipboard" | null>(null)
  const [draft, setDraft] = useState("")
  const [hintVisible, setHintVisible] = useState(true)

  const url = useMemo(
    () => (device.mode !== "direct" ? sessionUrl(device.id) : ""),
    [device.id, device.mode],
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

  // The hint is for the first few seconds of a session, not a permanent label
  // over the bottom of the screen.
  useEffect(() => {
    if (!connected || !hintVisible) return
    const timer = window.setTimeout(() => setHintVisible(false), 6000)
    return () => window.clearTimeout(timer)
  }, [connected, hintVisible])

  return (
    <div ref={shellRef} className="flex h-full min-h-0 flex-col bg-neutral-950">
      <div className="flex h-11 shrink-0 items-center gap-2 border-b border-white/10 px-2 text-neutral-200">
        {leading}
        <span className="flex min-w-0 items-center gap-1.5">
          <span className={cn("size-1.5 shrink-0 rounded-full", statusDot(status))} />
          <span className="truncate text-[12px] font-medium text-neutral-100">{device.name}</span>
          <span className="hidden truncate text-[10.5px] text-neutral-500 sm:inline">
            {statusLabel(status)}
          </span>
        </span>
        <div ref={toolbarRef} className="relative ml-auto flex items-center gap-0.5">
          <ToolbarButton disabled={!connected} onClick={focus} label="Focus the keyboard">
            <Keyboard />
          </ToolbarButton>
          <ToolbarButton
            disabled={!connected}
            expanded={panel === "keys"}
            onClick={() => togglePanel("keys")}
            label="Extra keys"
          >
            <Command />
          </ToolbarButton>
          <ToolbarButton
            disabled={!connected}
            expanded={panel === "clipboard"}
            onClick={() => togglePanel("clipboard")}
            label="Clipboard"
          >
            <Clipboard />
          </ToolbarButton>
          <ToolbarButton onClick={fullscreen} label="Fullscreen">
            <Maximize2 />
          </ToolbarButton>
          <ToolbarButton onClick={startOver} label="Reconnect">
            <RotateCcw />
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
      </div>

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
            <p className="text-sm text-neutral-300">Connecting to {device.name}…</p>
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
            <OutlineButton onClick={startOver}>
              <RotateCcw />
              Retry
            </OutlineButton>
          </Overlay>
        ) : null}
      </div>
    </div>
  )
}

function statusDot(status: VncStatus) {
  if (status === "connected") return "bg-emerald-400"
  if (status === "failed") return "bg-red-400"
  return "bg-amber-300 animate-pulse"
}

function statusLabel(status: VncStatus) {
  if (status === "connected") return "connected"
  if (status === "failed") return "disconnected"
  if (status === "reconnecting") return "reconnecting"
  return "connecting"
}

function ToolbarButton({
  disabled,
  expanded,
  onClick,
  label,
  children,
}: {
  disabled?: boolean
  expanded?: boolean
  onClick: () => void
  label: string
  children: ReactNode
}) {
  return (
    <Button
      size="icon-sm"
      variant="ghost"
      disabled={disabled}
      aria-expanded={expanded}
      aria-label={label}
      title={label}
      className="text-neutral-400 hover:bg-white/10 hover:text-white disabled:text-neutral-600"
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
