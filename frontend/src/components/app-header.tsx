import { LogOut, Plus } from "lucide-react"
import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export type HubStatus = {
  label: string
  tone: "online" | "offline" | "idle"
}

export function AppHeader({
  signedIn,
  status,
  onAdd,
  onLogout,
}: {
  signedIn: boolean
  status: HubStatus
  onAdd: () => void
  onLogout: () => void
}) {
  return (
    <header className="mx-auto flex w-full max-w-6xl items-center justify-between gap-4 px-5 py-6">
      <Link to="/" className="flex items-center gap-2.5" aria-label="abacad home">
        <span className="grid size-9 place-items-center rounded-xl bg-primary text-lg font-semibold text-primary-foreground shadow-sm">
          a
        </span>
        <span className="text-[15px] font-semibold tracking-tight">abacad</span>
      </Link>

      <div className="flex items-center gap-2">
        <span
          className={cn(
            "hidden items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium sm:inline-flex",
            status.tone === "online" &&
              "border-emerald-200 bg-emerald-50 text-emerald-800",
            status.tone === "offline" &&
              "border-red-200 bg-red-50 text-red-800",
            status.tone === "idle" &&
              "border-border bg-card text-muted-foreground",
          )}
        >
          <span
            className={cn(
              "size-1.5 rounded-full",
              status.tone === "online" && "bg-emerald-500",
              status.tone === "offline" && "bg-red-500",
              status.tone === "idle" && "bg-muted-foreground/50",
            )}
          />
          {status.label}
        </span>

        {signedIn ? (
          <>
            <Button size="sm" onClick={onAdd}>
              <Plus />
              Add device
            </Button>
            <Button size="sm" variant="ghost" onClick={onLogout}>
              <LogOut />
              <span className="hidden sm:inline">Sign out</span>
            </Button>
          </>
        ) : null}
      </div>
    </header>
  )
}
