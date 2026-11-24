import { KeyRound } from "lucide-react"
import { useState } from "react"

import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"

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
    <Card className="mx-auto w-full max-w-[400px] border-border/80 bg-card/90 shadow-xl shadow-foreground/5 backdrop-blur">
      <CardHeader className="gap-2">
        <div className="mb-1 grid size-11 place-items-center rounded-xl bg-accent text-accent-foreground">
          <KeyRound className="size-5" />
        </div>
        <CardTitle className="text-xl">Sign in</CardTitle>
        <CardDescription>
          Paste the admin token from <code className="font-mono text-xs">~/.config/abacad/config.yaml</code>.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault()
            onSubmit(token)
          }}
        >
          <Input
            autoFocus
            type="password"
            autoComplete="current-password"
            placeholder="admin token"
            value={token}
            onChange={(event) => setToken(event.target.value)}
            className="h-10 bg-background/70"
            required
          />
          <Button type="submit" className="h-10" disabled={busy || token.trim() === ""}>
            {busy ? "Unlocking…" : "Unlock"}
          </Button>
          {error ? (
            <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </p>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}
