import { Link } from "react-router-dom"

import { Button } from "@/components/ui/button"

export default function NotFound() {
  return (
    <div className="grid min-h-screen place-items-center p-6 text-center">
      <div className="space-y-3">
        <p className="font-mono text-xs tracking-[0.2em] text-muted-foreground uppercase">404</p>
        <h1 className="text-2xl font-medium tracking-tight">Nothing here</h1>
        <p className="text-sm text-muted-foreground">That page is not part of the hub.</p>
        <Button asChild variant="outline">
          <Link to="/">Back to devices</Link>
        </Button>
      </div>
    </div>
  )
}
