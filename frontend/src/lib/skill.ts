import { statusLabel, deviceStatus } from "@/lib/device"
import type { Device } from "@/lib/types"

// The agent skill: one SKILL.md that names the hub, carries the hub admin token and
// lists the machines. Every machine is reached the way the browser reaches it:
// a websockify-style WebSocket at /ws/vnc/{id} that carries plain RFB, through
// whatever HTTPS front the hub already has.

export const SKILL_FILENAME = "SKILL.md"

export function normalizeHub(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "")
  if (!trimmed) return ""
  return /^https?:\/\//i.test(trimmed) ? trimmed : `http://${trimmed}`
}

// The HTTP address of the hub: scheme, host and, if present, the web port.
export function hubOrigin(value: string): string {
  const normalized = normalizeHub(value)
  try {
    return new URL(normalized).origin
  } catch {
    return normalized
  }
}

// ws:// for an http hub, wss:// for an https one.
export function wsOrigin(value: string): string {
  return hubOrigin(value).replace(/^http/i, "ws")
}

export interface AgentEndpoint {
  device: Device
  // The WebSocket an agent opens. Empty for catalog-only devices.
  url: string
  // What the VNC handshake inside the WebSocket asks for.
  password: "none" | "device" | "direct"
}

export function agentEndpoints(devices: Device[], hub: string): AgentEndpoint[] {
  const ws = wsOrigin(hub)
  return devices.map((device) => {
    if (device.mode === "direct") {
      return { device, url: "", password: "direct" }
    }
    return {
      device,
      url: `${ws}/ws/vnc/${encodeURIComponent(device.id)}`,
      // gateway: the hub signs in to the device. l4: bytes pass through
      // untouched, so the client answers the device's own password.
      password: device.mode === "l4" ? "device" : "none",
    }
  })
}

function cell(value: string): string {
  return value.replace(/\|/g, "\\|")
}

function tableRow(endpoint: AgentEndpoint): string {
  const { device } = endpoint
  const state = statusLabel(deviceStatus(device))
  const where =
    endpoint.password === "direct"
      ? `not through the hub: \`${device.host}:${device.port}\``
      : `\`${endpoint.url}\``
  const password = endpoint.password === "none" ? "none" : "the machine's own — ask the user"
  return `| ${cell(device.name)} | \`${device.id}\` | ${cell(device.platform || "—")} | ${where} | ${password} | ${state} |`
}

function frontmatter(origin: string): string {
  return [
    "---",
    "name: abacad",
    `description: See and drive the real machines registered on the abacad VNC hub at ${origin}. Use when the user asks you to open a machine, look at another computer's or phone's screen, take a screenshot of a device, click, tap or type on it, or run something on a named computer (macOS, Linux, Windows, Android, iOS). Every machine is plain VNC (RFB) carried over a WebSocket, the websockify convention noVNC uses; any VNC client works through a local websocat bridge.`,
    "---",
    "",
  ].join("\n")
}

const CONNECT = [
  "## Connect",
  "",
  "Most VNC clients speak raw TCP, so bridge the WebSocket to a local port with",
  "[websocat](https://github.com/vi/websocat) (one static binary: `brew install",
  "websocat`, or a release download), then point any VNC client at `127.0.0.1`.",
  "",
  "```bash",
  "websocat --binary -H=\"Authorization: Bearer $ABACAD_TOKEN\" \\",
  "  tcp-l:127.0.0.1:5900 {url} &",
  "```",
  "",
  "Every TCP connection to the bridge is one VNC session on the machine. Keep",
  "**one session for the whole task**; reconnecting per action repeats the",
  "handshake with the device each time. With Python's `vncdotool` (use a venv",
  "if pip refuses to install system-wide):",
  "",
  "```python",
  "from vncdotool import api",
  "",
  "c = api.connect(\"127.0.0.1::5900\", password={password})  # host::port",
  "",
  "# look",
  "c.captureScreen(\"screen.png\")",
  "# click / tap",
  "c.mouseMove(200, 400); c.mousePress(1)",
  "# drag / swipe",
  "c.mouseMove(200, 700); c.mouseDown(1); c.mouseDrag(200, 300, step=20); c.mouseUp(1)",
  "# the API has no type(): press keys one by one",
  "for ch in \"hello\":",
  "    c.keyPress(ch)",
  "c.keyPress(\"enter\")",
  "",
  "c.disconnect()",
  "```",
  "",
  "If each of your shell commands runs in a fresh process, start that connection",
  "once as a long-running background process and send it commands (for example",
  "over a Unix socket) instead of reconnecting per action.",
  "",
  "Clients that speak WebSocket themselves skip the bridge: noVNC, or",
  "[py-wsvnc](https://pypi.org/project/py-wsvnc/) in Python with",
  "`WSVNCClient(ticket_url=\"{url}?token=$ABACAD_TOKEN\")`. py-wsvnc assumes one",
  "RFB message per WebSocket frame and can lose sync on large screen updates;",
  "prefer the bridge for real work.",
  "",
  "## Phones and tablets",
  "",
  "- A black frame usually means the screen is off. Tap once or press `home`, then",
  "  capture again.",
  "- Key events reach Android as a hardware keyboard and fight the input method;",
  "  non-ASCII text cannot be typed that way. For Chinese and other IME text, tap",
  "  the on-screen keyboard (for example type pinyin, then tap the candidate).",
  "- Coordinates are framebuffer pixels, which may be scaled from the physical",
  "  screen. Take them from the frame you just captured.",
  "",
]

const RULES = [
  "## Rules",
  "",
  "- Use only the machines the hub lists. Do not scan the LAN or probe ports.",
  "- Open one session per task and reuse it. Do not capture frames in a tight loop; one frame per action is enough.",
  "- A rejected password is a stop sign. Some VNC servers blacklist a client after repeated failures. If a machine rejects you twice, stop and ask the user.",
  "- A machine's state is a hint, not a probe. The hub only marks a machine `online` after a real session authenticated, and that mark goes stale after ten minutes. `not seen` usually still connects.",
  "- The token opens every machine on this hub and the hub's settings. Keep it out of chat, logs, commits and screenshots of your own terminal.",
  "",
]

export function buildSkill({
  devices,
  hub,
  hubToken,
}: {
  devices: Device[]
  hub: string
  hubToken: string
}): string {
  const origin = hubOrigin(hub)
  const endpoints = agentEndpoints(devices, hub)
  const sample = endpoints.find((endpoint) => endpoint.password === "none") ??
    endpoints.find((endpoint) => endpoint.url) ?? {
      url: `${wsOrigin(hub)}/ws/vnc/<id>`,
      password: "none" as const,
    }

  const connect = CONNECT.join("\n")
    .replaceAll("{url}", sample.url)
    .replaceAll("{password}", sample.password === "none" ? "None" : '"<the machine\'s own password>"')

  const lines = [
    frontmatter(origin),
    "# abacad",
    "",
    `\`${origin}\` is a VNC hub on the user's network. The machines below are real`,
    "desktops and phones; the hub is the only host that can reach them. Each one is",
    "a WebSocket on the hub that carries plain VNC (RFB), the same one the hub's web",
    "page uses.",
    "",
    "## Admin token",
    "",
    "One token opens everything on this hub: the API, the WebSockets and the web UI.",
    "",
    "```bash",
    `export ABACAD_TOKEN='${hubToken || "<ask the user for the abacad admin token>"}'`,
    "```",
    "",
    "- HTTP and WebSocket: `Authorization: Bearer $ABACAD_TOKEN`.",
    "- A WebSocket client that takes only a URL: append `?token=$ABACAD_TOKEN`.",
    "",
    "## Machines",
    "",
    "| machine | id | platform | WebSocket | VNC password | state |",
    "| --- | --- | --- | --- | --- | --- |",
    ...endpoints.map(tableRow),
    "",
  ]

  if (endpoints.length === 0) {
    lines.push("> The hub has no machines yet. Ask the user to add one in abacad.", "")
  }

  lines.push(
    "The list changes. Fetch the current one, with live state, before you rely on it:",
    "",
    "```bash",
    `curl -fsS -H "Authorization: Bearer $ABACAD_TOKEN" '${origin}/api/devices'`,
    "```",
    "",
    `A machine's WebSocket is \`${wsOrigin(hub)}/ws/vnc/<id>\`. \`mode: l4\` machines`,
    "pass VNC through untouched, so their own VNC password is asked inside the",
    "session; `mode: direct` machines are catalog entries the hub does not carry.",
    "",
    connect,
    ...RULES,
  )

  return lines.join("\n")
}
