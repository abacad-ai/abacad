/**
 * A phone's own buttons, reached over RFB.
 *
 * RFB carries keysyms, not buttons. A phone's global actions — home, lock, the
 * volume keys — are therefore only reachable through the shortcut chords its
 * VNC server listens for.
 *
 * On Android those are droidVNC-NG's defaults, and the device owner can rebind
 * them, so the table below is the one place that mapping is written down:
 * change a chord in the app, change it here. On iOS the server is iOS VNC
 * Bridge, which turns the same keysyms into hardware-button reports; its half
 * of the mapping lives in `internal/helper/keymap.go` in that repo.
 *
 * The keys mean nothing on macOS, Windows or Linux, which is why only phones
 * get the bar.
 */

interface KeySpec {
  key: string
  code: string
  keyCode: number
}

export interface DeviceKey {
  id: string
  label: string
  /** The chord the device's VNC server listens for, for the tooltip. */
  chord: string
  /** A divider separates buttons that belong to different parts of the phone. */
  section: "nav" | "system" | "volume"
  keys: KeySpec[]
}

const Ctrl: KeySpec = { key: "Control", code: "ControlLeft", keyCode: 17 }
const Shift: KeySpec = { key: "Shift", code: "ShiftLeft", keyCode: 16 }
const Escape: KeySpec = { key: "Escape", code: "Escape", keyCode: 27 }
const Home: KeySpec = { key: "Home", code: "Home", keyCode: 36 }
const End: KeySpec = { key: "End", code: "End", keyCode: 35 }
const PageUp: KeySpec = { key: "PageUp", code: "PageUp", keyCode: 33 }
const PageDown: KeySpec = { key: "PageDown", code: "PageDown", keyCode: 34 }

// From a Mac, noVNC deliberately sends XK_Mode_switch instead of XK_Alt_L, so
// that Option reaches RealVNC/TigerVNC as AltGraph. droidVNC-NG accepts that as
// Alt too ("MacOS clients send Alt as 0xFF7E"), so every browser sends the same
// plain Alt and the chord still matches on the device.
const Alt: KeySpec = { key: "Alt", code: "AltLeft", keyCode: 18 }

export const ANDROID_KEYS: DeviceKey[] = [
  { id: "back", label: "Back", chord: "Escape", section: "nav", keys: [Escape] },
  { id: "home", label: "Home", chord: "Home", section: "nav", keys: [Home] },
  {
    id: "recents",
    label: "Recents",
    chord: "Ctrl+Shift+Esc",
    section: "nav",
    keys: [Ctrl, Shift, Escape],
  },
  { id: "power", label: "Power menu", chord: "End", section: "system", keys: [End] },
  {
    id: "volume-down",
    label: "Volume down",
    chord: "Ctrl+Alt+PageDown",
    section: "volume",
    keys: [Ctrl, Alt, PageDown],
  },
  {
    id: "volume-up",
    label: "Volume up",
    chord: "Ctrl+Alt+PageUp",
    section: "volume",
    keys: [Ctrl, Alt, PageUp],
  },
]

// iOS has no Back (the system gesture is a swipe, not a key) and no Recents
// chord: SpringBoard's app switcher wants a double press of the side button or
// the Home button, which is a sequence rather than one RFB key event. Home and
// Lock are all the system buttons a keysym can reach.
export const IOS_KEYS: DeviceKey[] = [
  { id: "home", label: "Home", chord: "Home", section: "system", keys: [Home] },
  { id: "lock", label: "Lock", chord: "End", section: "system", keys: [End] },
  {
    id: "volume-down",
    label: "Volume down",
    chord: "Ctrl+Alt+PageDown",
    section: "volume",
    keys: [Ctrl, Alt, PageDown],
  },
  {
    id: "volume-up",
    label: "Volume up",
    chord: "Ctrl+Alt+PageUp",
    section: "volume",
    keys: [Ctrl, Alt, PageUp],
  },
]

/** The bar for a platform, or null for machines that have no phone buttons. */
export function deviceKeys(platform?: string): DeviceKey[] | null {
  switch (platform) {
    case "android":
      return ANDROID_KEYS
    case "ios":
    case "ipados":
      return IOS_KEYS
    default:
      return null
  }
}

function keyEvent(
  type: "keydown" | "keyup",
  spec: KeySpec,
  flags: { ctrlKey: boolean; altKey: boolean; shiftKey: boolean },
) {
  const event = new KeyboardEvent(type, {
    key: spec.key,
    code: spec.code,
    bubbles: true,
    cancelable: true,
    ...flags,
  })
  // KeyboardEventInit drops keyCode/which; noVNC's legacy fallbacks still read
  // them when a key has no code.
  Object.defineProperty(event, "keyCode", { get: () => spec.keyCode })
  Object.defineProperty(event, "which", { get: () => spec.keyCode })
  return event
}

/**
 * Hands one chord to the noVNC application in the frame. It is not a library —
 * it is a page — so the only way in without patching the vendored tree is to
 * feed its keyboard the events it already listens for on the screen canvas.
 *
 * Returns false when the frame has not drawn a screen yet (still connecting).
 */
export function sendDeviceKey(
  frame: HTMLIFrameElement | null,
  key: DeviceKey,
): boolean {
  const canvas = frame?.contentDocument?.querySelector<HTMLCanvasElement>(
    "#noVNC_container canvas",
  )
  if (!canvas) return false

  const modifiers = key.keys.slice(0, -1)
  const main = key.keys[key.keys.length - 1]
  const flags = {
    ctrlKey: key.keys.includes(Ctrl),
    altKey: key.keys.includes(Alt),
    shiftKey: key.keys.includes(Shift),
  }

  for (const modifier of modifiers) canvas.dispatchEvent(keyEvent("keydown", modifier, flags))
  canvas.dispatchEvent(keyEvent("keydown", main, flags))
  canvas.dispatchEvent(keyEvent("keyup", main, flags))
  for (const modifier of [...modifiers].reverse())
    canvas.dispatchEvent(keyEvent("keyup", modifier, flags))

  // The button took focus on mousedown; give it back so the physical keyboard
  // keeps typing into the device and not into our chrome.
  canvas.focus()
  return true
}
