import { ArrowLeft, Keyboard, Maximize2, RotateCcw } from "lucide-react"
import { useEffect, useMemo, useRef, useState } from "react"
import { Link, useNavigate, useParams } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { ApiError, api } from "@/lib/api"
import type { Device } from "@/lib/types"

function viewerUrl(id: string, nonce: number) {
  const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:"
  const params = new URLSearchParams({
    autoconnect: "1",
    path: `${wsProtocol}//${window.location.host}/ws/vnc/${encodeURIComponent(id)}`,
    resize: "scale",
    reconnect: "1",
    reconnect_delay: "1500",
    quality: "7",
    compression: "1",
    show_dot: "1",
    view_only: "0",
    _r: String(nonce),
  })
  return `/novnc/vnc.html?${params.toString()}`
}

export default function Viewer() {
  const { id = "" } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const frameRef = useRef<HTMLIFrameElement>(null)
  const frameContainerRef = useRef<HTMLDivElement>(null)
  const [device, setDevice] = useState<Device | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const [nonce, setNonce] = useState(0)
  const [hintVisible, setHintVisible] = useState(true)

  const src = useMemo(() => (id ? viewerUrl(id, nonce) : "about:blank"), [id, nonce])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError("")
    api
      .devices()
      .then((response) => {
        if (cancelled) return
        const found = response.devices.find((candidate) => candidate.id === id) ?? null
        if (!found) {
          setError("Device not found.")
          setLoading(false)
          return
        }
        if (found.mode === "direct") {
          setError(
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
        setError(caught instanceof Error ? caught.message : "Could not load the device.")
        setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [id, navigate])

  useEffect(() => {
    const frame = frameRef.current
    if (!frame || loading || error) return

    let tries = 0
    let timer: number | undefined

    const hideHint = () => setHintVisible(false)
    const focusCanvas = () => {
      try {
        const doc = frame.contentDocument
        const canvas = doc?.querySelector("canvas") as HTMLCanvasElement | null
        if (!canvas) return false
        canvas.focus()
        doc?.addEventListener("pointerdown", hideHint, { once: true })
        return true
      } catch {
        return false
      }
    }

    const onLoad = () => {
      setHintVisible(true)
      timer = window.setInterval(() => {
        tries += 1
        if (focusCanvas() || tries > 20) {
          if (timer) window.clearInterval(timer)
        }
      }, 300)
    }

    frame.addEventListener("load", onLoad)
    if (frame.contentDocument?.readyState === "complete") onLoad()

    return () => {
      frame.removeEventListener("load", onLoad)
      if (timer) window.clearInterval(timer)
    }
  }, [src, loading, error])

  function focusRemoteKeyboard() {
    try {
      const doc = frameRef.current?.contentDocument
      const canvas = doc?.querySelector("canvas") as HTMLCanvasElement | null
      canvas?.focus()
      const keyboardButton = doc?.getElementById("noVNC_keyboard_button") as HTMLElement | null
      if (keyboardButton && !keyboardButton.classList.contains("noVNC_selected")) {
        keyboardButton.click()
      }
      const input = doc?.getElementById("noVNC_keyboardinput") as HTMLElement | null
      input?.focus()
    } catch {
      // Cross-origin would be a deployment bug; the iframe is same-origin.
    }
    setHintVisible(false)
  }

  function reconnect() {
    setNonce((value) => value + 1)
  }

  function fullscreen() {
    const request = frameContainerRef.current?.requestFullscreen?.()
    void request?.catch(() => {})
  }

  if (loading) {
    return (
      <div className="grid min-h-screen place-items-center bg-neutral-950 text-sm text-neutral-300">
        Connecting…
      </div>
    )
  }

  if (error) {
    return (
      <div className="grid min-h-screen place-items-center p-6">
        <Card className="max-w-md items-center gap-3 p-6 text-center">
          <p className="text-base font-medium">Can’t open this device</p>
          <p className="text-sm text-muted-foreground">{error}</p>
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
    <div className="flex h-screen flex-col bg-neutral-950">
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-white/10 bg-neutral-950 px-2 text-neutral-200">
        <Button asChild size="sm" variant="ghost" className="text-neutral-300 hover:bg-white/10 hover:text-white">
          <Link to="/">
            <ArrowLeft />
            Hub
          </Link>
        </Button>
        <span className="min-w-0 truncate font-mono text-xs text-neutral-400">
          {device?.name || id}
        </span>
        <div className="ml-auto flex items-center gap-1">
          <Button
            size="sm"
            variant="ghost"
            className="text-neutral-300 hover:bg-white/10 hover:text-white"
            onClick={focusRemoteKeyboard}
          >
            <Keyboard />
            <span className="hidden sm:inline">Keyboard</span>
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-neutral-300 hover:bg-white/10 hover:text-white"
            onClick={reconnect}
          >
            <RotateCcw />
            <span className="hidden sm:inline">Reconnect</span>
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="text-neutral-300 hover:bg-white/10 hover:text-white"
            onClick={fullscreen}
          >
            <Maximize2 />
            <span className="hidden sm:inline">Fullscreen</span>
          </Button>
        </div>
      </header>

      <div ref={frameContainerRef} className="relative min-h-0 flex-1">
        <iframe
          ref={frameRef}
          src={src}
          title="VNC session"
          className="h-full w-full border-0"
          allow="clipboard-read; clipboard-write; fullscreen"
        />
        {hintVisible ? (
          <div className="pointer-events-none absolute inset-x-0 bottom-5 mx-auto w-fit max-w-[90%] rounded-full border border-white/10 bg-black/70 px-4 py-2 text-center text-xs text-white/80 backdrop-blur">
            Click the screen, then type · macOS lock screen: press Space or click your user
          </div>
        ) : null}
      </div>
    </div>
  )
}
