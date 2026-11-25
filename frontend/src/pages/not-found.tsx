import { Link } from "react-router-dom"

import { BrandMark } from "@/components/brand-mark"
import { Button } from "@/components/ui/button"

export default function NotFound() {
  return (
    <div className="grid min-h-screen place-items-center bg-background p-6 text-center">
      <div className="flex flex-col items-center">
        <BrandMark className="size-8" />
        <p className="mt-6 font-mono text-[10px] tracking-[0.18em] text-muted-foreground uppercase">
          404
        </p>
        <h1 className="mt-2 text-[24px] font-semibold tracking-[-0.035em]">Nothing here</h1>
        <p className="mt-2 text-[13px] text-muted-foreground">That page is not part of the hub.</p>
        <Button asChild variant="outline" className="mt-6 h-8 px-3 text-[12px]">
          <Link to="/">Back to devices</Link>
        </Button>
      </div>
    </div>
  )
}
