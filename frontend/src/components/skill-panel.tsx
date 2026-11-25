import { Bot, Copy, Download, ShieldAlert } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { buildSkill, normalizeHub, SKILL_FILENAME } from "@/lib/skill"
import type { Device } from "@/lib/types"

// Remember the address the human typed so a reverse proxy or a second LAN name
// does not have to be retyped on every visit.
const HUB_KEY = "abacad.skill.hub"

export function SkillPanel({ devices }: { devices: Device[] }) {
  const [hubToken, setHubToken] = useState("")
  const [hub, setHub] = useState(() => {
    if (typeof window === "undefined") return ""
    return window.localStorage.getItem(HUB_KEY) || window.location.origin
  })

  useEffect(() => {
    const normalized = normalizeHub(hub)
    if (normalized) window.localStorage.setItem(HUB_KEY, normalized)
  }, [hub])

  // The page is behind the signed-in session; the admin token is baked into the file so
  // the agent needs nothing else.
  useEffect(() => {
    let cancelled = false
    api
      .token()
      .then((response) => {
        if (!cancelled) setHubToken(response.token)
      })
      .catch(() => {
        if (!cancelled) toast.error("Could not read the admin token")
      })
    return () => {
      cancelled = true
    }
  }, [])

  const markdown = useMemo(() => buildSkill({ devices, hub, hubToken }), [devices, hub, hubToken])

  async function copy() {
    const copied = await copyText(markdown)
    if (copied) toast.success("Skill copied")
    else toast.error("Could not copy")
  }

  function download() {
    const blob = new Blob([markdown], { type: "text/markdown;charset=utf-8" })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement("a")
    anchor.href = url
    anchor.download = SKILL_FILENAME
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    toast.success(`${SKILL_FILENAME} downloaded`)
  }

  return (
    <div className="min-h-0 flex-1 overflow-y-auto bg-background">
      <div className="mx-auto w-full max-w-3xl px-5 py-9 sm:px-8 lg:py-12">
        <p className="flex items-center gap-2 font-mono text-[10px] tracking-[0.16em] text-muted-foreground uppercase">
          <Bot className="size-3" />
          For agents
        </p>
        <h1 className="mt-3 text-[26px] leading-tight font-semibold tracking-[-0.04em]">
          Hand a machine to an agent
        </h1>
        <p className="mt-2.5 max-w-[62ch] text-[13px] leading-relaxed text-muted-foreground">
          This is a ready-made agent skill: the machine list, the endpoints and the rules that keep
          the hub healthy. Save it as{" "}
          <code className="rounded-[4px] border border-border bg-card px-1 py-px font-mono text-[11px] text-foreground/80">
            SKILL.md
          </code>{" "}
          in your agent&apos;s skills folder. No MCP server and no new protocol — every machine is
          plain VNC over the same WebSocket this page uses.
        </p>

        <p className="mt-4 font-mono text-[10.5px] tracking-[0.04em] text-muted-foreground">
          {devices.length === 0
            ? "No machines yet"
            : `${devices.length} machine${devices.length === 1 ? "" : "s"}`}
          <span className="text-faint"> · </span>
          {normalizeHub(hub)}
        </p>

        <div className="mt-5 overflow-hidden rounded-xl border border-border bg-card shadow-card">
          <div className="flex flex-wrap items-center gap-2 border-b border-border px-2.5 py-2.5">
            <input
              value={hub}
              onChange={(event) => setHub(event.target.value)}
              spellCheck={false}
              aria-label="Hub address"
              className="h-7 min-w-[160px] flex-1 rounded-md border border-input bg-background px-2.5 font-mono text-[11px] text-foreground/80 outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40"
            />
            <Button size="sm" variant="outline" onClick={() => void copy()}>
              <Copy className="size-3.5" />
              Copy
            </Button>
            <Button size="sm" onClick={download}>
              <Download className="size-3.5" />
              Download
            </Button>
          </div>

          <pre className="h-[46vh] min-h-[280px] overflow-x-hidden overflow-y-auto bg-background px-4 py-4 font-mono text-[11px] leading-[1.75] break-words whitespace-pre-wrap text-foreground/75">
            {markdown}
          </pre>

          <div className="flex items-start gap-2.5 border-t border-border px-4 py-3">
            <ShieldAlert className="mt-0.5 size-3.5 shrink-0 text-faint" />
            <p className="text-[11px] leading-relaxed text-muted-foreground">
              This file contains the admin token, which opens every machine and the hub&apos;s
              settings. Treat it like a password and keep it out of git.
            </p>
          </div>
        </div>

        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <div className="rounded-xl border border-border bg-card px-4 py-3">
            <p className="font-mono text-[9.5px] tracking-[0.14em] text-muted-foreground uppercase">
              Where it goes
            </p>
            <p className="mt-2 text-[12px] leading-relaxed text-muted-foreground">
              One folder per skill:
              <br />
              <code className="font-mono text-[11px] text-foreground/80">
                ~/.claude/skills/abacad/SKILL.md
              </code>
              <br />
              <code className="font-mono text-[11px] text-foreground/80">
                ~/.dsh/skills/abacad/SKILL.md
              </code>
            </p>
          </div>
          <div className="rounded-xl border border-border bg-card px-4 py-3">
            <p className="font-mono text-[9.5px] tracking-[0.14em] text-muted-foreground uppercase">
              How it connects
            </p>
            <p className="mt-2 text-[12px] leading-relaxed text-muted-foreground">
              Each machine is a WebSocket at{" "}
              <code className="font-mono text-[11px] text-foreground/80">/ws/vnc/&lt;id&gt;</code>.
              The agent bridges it to a local port with websocat and uses any VNC client.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
