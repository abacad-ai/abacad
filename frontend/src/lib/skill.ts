import { statusLabel, deviceStatus } from "@/lib/device"
import type { Device } from "@/lib/types"

// Two ways to hand a machine to an agent. "baked" is a static file that already
// lists every endpoint and token, so the agent needs no access to the hub API.
// "live" makes the agent fetch the list each run, which stays correct when
// machines or tokens change but requires the hub's admin token.
export type SkillVariant = "baked" | "live"

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

// Raw VNC ports live on the hub host but not on the hub's web port, so an
// endpoint is host:59xx, never host:8787:59xx.
export function hubHost(value: string): string {
  try {
    return new URL(normalizeHub(value)).hostname
  } catch {
    return normalizeHub(value)
      .replace(/^https?:\/\//i, "")
      .replace(/[/:].*$/, "")
  }
}

export interface AgentEndpoint {
  device: Device
  // host:port the agent's VNC client dials. Empty when there is no endpoint.
  address: string
  // null means the hub does not hold this password; the user has to supply it.
  password: string | null
  usable: boolean
  note: string
}

export function agentEndpoints(devices: Device[], hub: string): AgentEndpoint[] {
  const host = hubHost(hub)
  return devices.map((device) => {
    if (device.mode === "direct") {
      return {
        device,
        address: `${device.host}:${device.port}`,
        password: null,
        usable: true,
        note: "catalog only — the hub carries no traffic, connect to the device itself",
      }
    }
    if (!device.vnc_port) {
      return {
        device,
        address: "",
        password: null,
        usable: false,
        note: "browser only — no raw VNC port",
      }
    }
    const address = `${host}:${device.vnc_port}`
    if (device.mode === "l4") {
      return {
        device,
        address,
        password: null,
        usable: true,
        note: "l4 — the device's own VNC password",
      }
    }
    if (!device.token) {
      return {
        device,
        address,
        password: null,
        usable: true,
        note: "gateway — no token on this device yet",
      }
    }
    return {
      device,
      address,
      password: device.token,
      usable: true,
      note: "gateway — token issued by the hub",
    }
  })
}

function cell(value: string): string {
  return value.replace(/\|/g, "\\|")
}

function tableRow(endpoint: AgentEndpoint): string {
  const state = statusLabel(deviceStatus(endpoint.device))
  const password = endpoint.password ? `\`${endpoint.password}\`` : "_ask the user_"
  return `| ${cell(endpoint.device.name)} | \`${endpoint.address}\` | ${password} | ${state} |`
}

// vncdotool takes host::port, so "10.0.0.4:5901" becomes "10.0.0.4::5901".
function vncdotoolAddress(address: string): string {
  return address.replace(/^(.*):(\d+)$/, "$1::$2")
}

function frontmatter(origin: string, how: string): string {
  return [
    "---",
    "name: abacad",
    `description: See and drive the real machines registered on this abacad VNC hub at ${origin}. Use when the user asks you to open a machine, look at another computer's screen, take a screenshot of a device, click or type on it, or run something on a named computer (macOS, Linux, Windows, Android, iOS). ${how} Every machine is a standard VNC (RFB) endpoint; no MCP server is involved and no project-specific API has to be learned.`,
    "---",
    "",
  ].join("\n")
}

const RULES = [
  "## Rules",
  "",
  "- Use only the machines the hub reports. Do not scan the LAN or probe ports on the hub.",
  "- Open one session per task and reuse it. Do not capture frames in a tight loop; one frame per action is enough.",
  "- A rejected password is a stop sign. The hub rate-limits failed handshakes and some VNC servers blacklist a client IP after repeated failures. If an endpoint rejects you twice, stop and ask the user.",
  "- A machine's state is a hint, not a probe. The hub only marks a machine `online` after a real session authenticated, and that mark goes stale after ten minutes. `not seen` usually still connects.",
  "- Treat the password column as a secret: do not paste it into chat, logs, commits or screenshots of your own terminal.",
  "",
]

const CONNECT = [
  "## Connect",
  "",
  "Any VNC client or library works. With Python's `vncdotool`:",
  "",
  "```bash",
  "python3 -m pip install --quiet vncdotool",
  "",
  "# one frame",
  "vncdotool -s {address} -p '{password}' capture screen.png",
  "",
  "# pointer and keyboard, same session flags",
  "vncdotool -s {address} -p '{password}' move 640 400 click 1",
  "vncdotool -s {address} -p '{password}' type 'hello'",
  "vncdotool -s {address} -p '{password}' key enter",
  "```",
  "",
  "`vncviewer {hostport}` and macOS Screen Sharing (`vnc://{hostport}`) connect the same way.",
  "The port belongs to the hub; you never need the device's own address.",
  "",
]

export function buildSkill({
  devices,
  hub,
  variant,
}: {
  devices: Device[]
  hub: string
  variant: SkillVariant
}): string {
  const origin = hubOrigin(hub)
  const host = hubHost(hub)
  const endpoints = agentEndpoints(devices, hub)
  const usable = endpoints.filter((endpoint) => endpoint.usable)
  const browserOnly = endpoints.filter((endpoint) => !endpoint.usable)

  if (variant === "live") {
    return [
      frontmatter(origin, "The list is fetched from the hub at run time."),
      "# abacad",
      "",
      `\`${origin}\` is a small VNC hub on this machine's LAN. It is the only host`,
      "that can reach the machines below, so you connect to the hub and it forwards",
      "you to the device. The machine list changes, so fetch it instead of trusting",
      "an old copy.",
      "",
      "## Credentials",
      "",
      "The hub's admin token must be in the environment as `ABACAD_TOKEN`. Ask the",
      "user to set it; never write it to a file or into the conversation.",
      "",
      "```bash",
      "jar=$(mktemp)",
      `curl -fsS -c "$jar" -X POST '${origin}/api/login' \\`,
      "  -H 'content-type: application/json' \\",
      '  -d "{\\"token\\":\\"$ABACAD_TOKEN\\"}" > /dev/null',
      `curl -fsS -b "$jar" '${origin}/api/devices'`,
      "```",
      "",
      "## Reading the answer",
      "",
      "- `host` / `port` — where the VNC server really is. Usually not reachable from",
      "  where you run; the hub is.",
      "- `vnc_port` — a port on the hub that forwards to that machine. Absent means",
      "  browser only: ask the user to enable a raw VNC port for agents.",
      `- Connect to \`${host}:<vnc_port>\` with \`token\` as the VNC password (\`mode: gateway\`).`,
      "  In `mode: l4` the password is the machine's own; in `mode: direct` there is",
      "  no hub port and you dial `host:port` directly.",
      "- `online` is \"seen in the last ten minutes\", not a live check.",
      "",
      ...RULES,
    ].join("\n")
  }

  const sample = usable.find((endpoint) => endpoint.password) ?? usable[0]
  const hostport = sample ? sample.address : `${host}:5901`
  const address = vncdotoolAddress(hostport)
  const password = sample?.password ?? "<password>"

  const connect = CONNECT.join("\n")
    .replaceAll("{address}", address)
    .replaceAll("{hostport}", hostport)
    .replaceAll("{password}", password)

  const lines = [
    frontmatter(origin, "A fixed list of endpoints is included below."),
    "# abacad",
    "",
    `\`${origin}\` is a small VNC hub on this machine's LAN. The machines below are`,
    "real desktops and phones; the hub is the only host that can reach them, so you",
    "connect to the hub and it forwards you to the device.",
    "",
    "| machine | endpoint | password | state |",
    "| --- | --- | --- | --- |",
    ...usable.map(tableRow),
    "",
  ]

  if (usable.length === 0) {
    lines.push(
      "> No machine currently has a raw VNC port. Ask the user to edit a machine in",
      "> abacad and enable \"open a raw VNC port for agents\".",
      "",
    )
  }

  if (browserOnly.length > 0) {
    lines.push(
      "Not available to you (browser only, no raw VNC port): " +
        browserOnly.map((endpoint) => cell(endpoint.device.name)).join(", ") +
        ". Ask the user to enable a raw VNC port if a task needs them.",
      "",
    )
  }

  const needsOwnPassword = usable.some((endpoint) => !endpoint.password)
  lines.push(connect)
  if (needsOwnPassword) {
    lines.push(
      "For rows whose password is \"ask the user\", the hub does not store that",
      "machine's VNC password. Ask for it before connecting; it is the machine's own.",
      "",
    )
  }
  lines.push(...RULES)

  return lines.join("\n")
}
