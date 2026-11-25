import { Bot, Copy, Download, KeyRound, ShieldAlert } from "lucide-react"
import { useEffect, useMemo, useState } from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { copyText } from "@/lib/clipboard"
import {
  agentEndpoints,
  buildSkill,
  normalizeHub,
  SKILL_FILENAME,
  type SkillVariant,
} from "@/lib/skill"
import type { Device } from "@/lib/types"
import { cn } from "@/lib/utils"

// Remember the address the human typed so a reverse proxy or a second LAN name
// does not have to be retyped on every visit.
const HUB_KEY = "abacad.skill.hub"

const VARIANTS: { id: SkillVariant; label: string; hint: string }[] = [
  {
    id: "baked",
    label: "Fixed list",
    hint: "The machines, endpoints and tokens are baked into the file. Nothing else to set up.",
  },
  {
    id: "live",
    label: "Live API",
    hint: "The agent fetches the list from the hub each run, so new machines appear.",
  },
]

export function SkillPanel({ devices }: { devices: Device[] }) {
  const [variant, setVariant] = useState<SkillVariant>("baked")
  const [hub, setHub] = useState(() => {
    if (typeof window === "undefined") return ""
    return window.localStorage.getItem(HUB_KEY) || window.location.origin
  })

  useEffect(() => {
    const normalized = normalizeHub(hub)
    if (normalized) window.localStorage.setItem(HUB_KEY, normalized)
  }, [hub])

  const endpoints = useMemo(() => agentEndpoints(devices, hub), [devices, hub])
  const ready = endpoints.filter((endpoint) => endpoint.usable)
  const markdown = useMemo(() => buildSkill({ devices, hub, variant }), [devices, hub, variant])
  const active = VARIANTS.find((option) => option.id === variant) ?? VARIANTS[0]

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
          in your agent&apos;s skills folder. No MCP server and no new protocol — every machine is a
          standard VNC endpoint.
        </p>

        {ready.length === 0 ? (
          <div className="mt-6 flex gap-3 rounded-xl border border-brand/25 bg-brand-soft px-4 py-3">
            <KeyRound className="mt-0.5 size-3.5 shrink-0 text-brand-ink" />
            <p className="text-[12.5px] leading-relaxed text-foreground/80">
              No machine has a raw VNC port yet, so there is nothing an agent can connect to. Edit a
              machine and check <strong className="font-medium text-foreground">Also open a raw VNC port for agents</strong>{" "}
              — that is what creates the endpoint.
            </p>
          </div>
        ) : (
          <p className="mt-4 font-mono text-[10.5px] tracking-[0.04em] text-muted-foreground">
            {ready.length === devices.length
              ? `${ready.length} machine${ready.length === 1 ? "" : "s"} ready`
              : `${ready.length} of ${devices.length} machines ready`}
            <span className="text-faint"> · </span>
            {normalizeHub(hub)}
          </p>
        )}

        <div className="mt-5 overflow-hidden rounded-xl border border-border bg-card shadow-card">
          <div className="flex flex-wrap items-center gap-2 border-b border-border px-2.5 py-2.5">
            <div className="flex rounded-lg bg-muted p-0.5">
              {VARIANTS.map((option) => (
                <button
                  key={option.id}
                  type="button"
                  onClick={() => setVariant(option.id)}
                  aria-pressed={variant === option.id}
                  className={cn(
                    "h-6 rounded-md px-2.5 text-[11.5px] font-medium transition-colors",
                    variant === option.id
                      ? "bg-card text-foreground shadow-pill"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {option.label}
                </button>
              ))}
            </div>
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
            {variant === "baked" ? (
              <ShieldAlert className="mt-0.5 size-3.5 shrink-0 text-faint" />
            ) : (
              <KeyRound className="mt-0.5 size-3.5 shrink-0 text-faint" />
            )}
            <p className="text-[11px] leading-relaxed text-muted-foreground">
              {variant === "baked" ? (
                ready.length === 0 ? (
                  <>
                    This file has no endpoints yet. Enable a raw VNC port on a machine and download
                    it again.
                  </>
                ) : (
                  <>
                    This file contains the VNC token for every machine listed. Treat it like a
                    password and keep it out of git. Rotate a token by editing the machine.
                  </>
                )
              ) : (
                <>
                  The live variant authenticates with the hub&apos;s admin token from{" "}
                  <code className="font-mono text-foreground/75">ABACAD_TOKEN</code> — that token
                  controls every machine on the hub. The file itself holds no secret.
                </>
              )}
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
              {active.label}
            </p>
            <p className="mt-2 text-[12px] leading-relaxed text-muted-foreground">{active.hint}</p>
          </div>
        </div>
      </div>
    </div>
  )
}
