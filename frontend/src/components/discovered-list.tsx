import { Plus } from "lucide-react"

import { Button } from "@/components/ui/button"
import type { DiscoveredDevice } from "@/lib/types"

export function DiscoveredList({
  entries,
  onAdd,
}: {
  entries: DiscoveredDevice[]
  onAdd: (entry: DiscoveredDevice) => void
}) {
  return (
    <div className="mt-3 overflow-hidden rounded-2xl border border-border/80 bg-card/80">
      {entries.map((entry) => (
        <div
          key={`${entry.host}:${entry.port}`}
          className="flex items-center justify-between gap-3 border-b border-border/70 px-4 py-3 last:border-b-0"
        >
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">{entry.name || "VNC server"}</p>
            <p className="truncate font-mono text-xs text-muted-foreground">
              {entry.host}:{entry.port}
            </p>
          </div>
          <Button size="sm" variant="outline" onClick={() => onAdd(entry)}>
            <Plus />
            Add
          </Button>
        </div>
      ))}
    </div>
  )
}
