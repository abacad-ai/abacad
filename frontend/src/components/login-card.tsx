import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

export function LoginCard({
  busy,
  error,
  onSubmit,
}: {
  busy: boolean
  error: string
  onSubmit: (token: string) => void
}) {
  const [token, setToken] = useState("")

  return (
    <Card className="mt-7 gap-0 rounded-2xl p-0">
      <CardContent className="p-5">
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault()
            onSubmit(token)
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="admin-token" className="text-[12px] font-medium text-foreground">
              Admin token
            </Label>
            <Input
              id="admin-token"
              autoFocus
              type="password"
              autoComplete="current-password"
              placeholder="paste your admin token"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              className="h-10 bg-background font-mono text-[12.5px]"
              required
            />
            <p className="text-[11.5px] leading-relaxed text-muted-foreground">
              From{" "}
              <code className="rounded-[4px] border border-border bg-background px-1 py-px font-mono text-[11px] text-foreground/75">
                ~/.config/abacad/config.yaml
              </code>{" "}
              on the machine running the hub.
            </p>
          </div>

          <Button type="submit" className="h-10 w-full text-[12.5px]" disabled={busy || token.trim() === ""}>
            {busy ? "Unlocking…" : "Unlock"}
          </Button>

          {error ? (
            <p
              role="alert"
              className="rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2 text-[12px] text-destructive"
            >
              {error}
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
