import RFB, { type RFBCredentials } from "@novnc/novnc"
import { useCallback, useEffect, useRef, useState } from "react"

// noVNC's own reconnection loop retried every 1.5s forever, which hammers a
// machine that is rebooting and hammers the hub while a device is offline.
// Back off instead, and stop entirely once the device refuses our credentials.
const BASE_RETRY_MS = 1500
const MAX_RETRY_MS = 15_000

// X11 keysyms for the modifier keys the extra-keys panel can hold down.
const MODIFIERS = {
  ctrl: { keysym: 0xffe3, code: "ControlLeft" },
  alt: { keysym: 0xffe9, code: "AltLeft" },
  meta: { keysym: 0xffeb, code: "MetaLeft" },
} as const

export type Modifier = keyof typeof MODIFIERS
export type VncStatus = "connecting" | "connected" | "reconnecting" | "failed"

const RELEASED: Record<Modifier, boolean> = { ctrl: false, alt: false, meta: false }

// The hub reports a device it cannot reach as an RFB security failure (it has
// to answer the handshake on the device's behalf), so a security failure alone
// does not mean "wrong password". Only give up when the reason is about
// credentials or a blacklist; those get worse if we keep retrying.
const AUTH_FAILURE = /authentication failed|too many security failures|invalid (password|credential)/i

export function useVncSession(url: string) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const rfbRef = useRef<RFB | null>(null)
  const attemptRef = useRef(0)
  const blockedRef = useRef(false)

  const [generation, setGeneration] = useState(0)
  const [status, setStatus] = useState<VncStatus>("connecting")
  const [error, setError] = useState("")
  const [clipboard, setClipboard] = useState("")
  const [modifiers, setModifiers] = useState<Record<Modifier, boolean>>(RELEASED)
  // Only l4 devices see this: the hub forwards the device's own RFB handshake,
  // so the browser has to answer it.
  const [credentialsRequest, setCredentialsRequest] = useState<string[] | null>(null)

  useEffect(() => {
    const container = containerRef.current
    if (!container || !url) return

    let disposed = false
    let retryTimer: number | undefined

    const drop = () => {
      const rfb = rfbRef.current
      rfbRef.current = null
      if (!rfb) {
        container.replaceChildren()
        return
      }
      try {
        rfb.disconnect()
      } catch {
        // Already torn down; the canvas is all that is left to clean up.
      }
      container.replaceChildren()
    }

    const retry = () => {
      const delay = Math.min(BASE_RETRY_MS * 2 ** attemptRef.current, MAX_RETRY_MS)
      attemptRef.current += 1
      setStatus("reconnecting")
      drop()
      retryTimer = window.setTimeout(start, delay)
    }

    const start = () => {
      if (disposed) return
      let rfb: RFB
      try {
        rfb = new RFB(container, url, { shared: true })
      } catch (caught) {
        setStatus("failed")
        setError(caught instanceof Error ? caught.message : "Could not start the session.")
        return
      }
      rfbRef.current = rfb
      rfb.scaleViewport = true
      rfb.clipViewport = false
      rfb.qualityLevel = 7
      rfb.compressionLevel = 1
      rfb.showDotCursor = true
      rfb.viewOnly = false
      rfb.background = "rgb(10, 10, 10)"

      // Every handler ignores events from a session that has already been
      // replaced, otherwise a late disconnect would undo the retry we scheduled.
      const own = () => !disposed && rfbRef.current === rfb

      rfb.addEventListener("connect", () => {
        if (!own()) return
        attemptRef.current = 0
        blockedRef.current = false
        setCredentialsRequest(null)
        setStatus("connected")
        setError("")
      })

      rfb.addEventListener("clipboard", (event) => {
        if (!own()) return
        setClipboard(event.detail.text)
      })

      rfb.addEventListener("securityfailure", (event) => {
        if (!own()) return
        const reason = event.detail.reason?.trim() || "The device rejected the connection."
        setError(reason)
        if (!AUTH_FAILURE.test(reason)) return
        blockedRef.current = true
        setStatus("failed")
      })

      rfb.addEventListener("credentialsrequired", (event) => {
        if (!own()) return
        setCredentialsRequest(event.detail.types)
      })

      // noVNC reports a server-side close as "clean" (it only clears that flag
      // when it fails mid-handshake), so the flag cannot tell "the user hung up"
      // from "the device went away". Our own teardowns are ignored above and
      // every other disconnect is worth retrying.
      rfb.addEventListener("disconnect", () => {
        if (!own()) return
        setModifiers(RELEASED)
        setCredentialsRequest(null)
        if (blockedRef.current) return
        retry()
      })
    }

    start()

    return () => {
      disposed = true
      window.clearTimeout(retryTimer)
      drop()
    }
  }, [url, generation])

  const reconnect = useCallback(() => {
    attemptRef.current = 0
    blockedRef.current = false
    setStatus("connecting")
    setGeneration((value) => value + 1)
  }, [])

  const sendCredentials = useCallback((credentials: RFBCredentials) => {
    rfbRef.current?.sendCredentials(credentials)
    setCredentialsRequest(null)
  }, [])

  const focus = useCallback(() => {
    rfbRef.current?.focus()
  }, [])

  const paste = useCallback((text: string) => {
    if (!text) return
    rfbRef.current?.clipboardPasteFrom(text)
  }, [])

  const toggleModifier = useCallback((name: Modifier) => {
    const rfb = rfbRef.current
    if (!rfb) return
    const next = !modifiers[name]
    rfb.sendKey(MODIFIERS[name].keysym, MODIFIERS[name].code, next)
    setModifiers((current) => ({ ...current, [name]: next }))
  }, [modifiers])

  const sendKey = useCallback((keysym: number, code: string) => {
    rfbRef.current?.sendKey(keysym, code)
  }, [])

  const sendCtrlAltDel = useCallback(() => {
    rfbRef.current?.sendCtrlAltDel()
  }, [])

  return {
    containerRef,
    status,
    error,
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
  }
}
