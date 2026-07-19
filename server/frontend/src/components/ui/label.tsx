import * as React from "react";
import { cn } from "@/lib/utils";

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn("font-mono text-[11px] font-medium uppercase tracking-[0.18em] text-ink-muted", className)}
      {...props}
    />
  );
}
