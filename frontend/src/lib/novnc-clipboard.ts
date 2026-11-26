/**
 * Clipboard, across the frame.
 *
 * The screen is noVNC's own application in an iframe, so its clipboard panel is
 * the bridge between this machine's clipboard and the device's — the same
 * viewer PVE and ESXi ship, and deliberately the same behaviour: Cmd/Ctrl+V
 * over the screen stays a keystroke for the device, never a local paste. Text
 * crosses as `ClientCutText` only, from the panel.
 *
 * It is worth writing down why no key is ever replayed here. A synthetic paste
 * has to land after the text does, and with an extended clipboard (RealVNC,
 * TigerVNC 1.15) the text does not travel with the first message: noVNC sends
 * a NOTIFY, the server asks for the text, and a PROVIDE carries it a round trip
 * later — 150 ms in practice. A replayed key races that, pastes the previous
 * clipboard, and types a bare "v" if the human has let go of the modifier by
 * then. Keeping the panel as the only path keeps that whole class of bug out.
 *
 * One detail of the panel does read as "clipboard is broken": its textarea
 * sends on `change`, which a browser fires on blur. Paste a URL into it, click
 * nothing else, and the device never sees it — the textarea merely shows the
 * text. So send a pasted value as soon as it lands; typing still goes through
 * noVNC's own change-on-blur, untouched.
 *
 * Nothing here touches the vendored tree (docs/vendored-novnc.md); it drives
 * the `change` event the application's own UI already listens for.
 */

const TEXTAREA_ID = "noVNC_clipboard_text"

/**
 * Makes the clipboard panel in `frame` send what is pasted into it right away.
 * Safe to call once per loaded frame; the returned function detaches.
 */
export function attachClipboardPanel(frame: HTMLIFrameElement): () => void {
  const loaded = frame.contentDocument
  const realmWindow = frame.contentWindow
  if (!loaded || !realmWindow) return () => {}
  // Named apart from the nullable reads above: the listener below closes over
  // them, and a narrowed binding is not narrowed inside a closure.
  const doc: Document = loaded
  // The frame's own constructors, so events are created in the realm that
  // dispatches them.
  const win = realmWindow as Window & typeof globalThis

  function onPaste(event: ClipboardEvent) {
    const target = event.target as Element | null
    if (!target || target.id !== TEXTAREA_ID) return
    // The browser writes the textarea just after this event, so tell noVNC it
    // changed then rather than waiting for the box to lose focus.
    win.setTimeout(() => {
      if (!doc.documentElement.classList.contains("noVNC_connected")) return
      target.dispatchEvent(new win.Event("change", { bubbles: true }))
    }, 0)
  }

  doc.addEventListener("paste", onPaste, true)
  return () => doc.removeEventListener("paste", onPaste, true)
}
