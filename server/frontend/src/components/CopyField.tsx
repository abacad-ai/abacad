import { useState } from "react";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils";

// CopyField shows a monospace secret/URL with a copy button. Used for tokens,
// wss URLs, and the `claude mcp add` snippet.
export function CopyField({ value, className }: { value: string; className?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div
      className={cn(
        "flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-900/70 p-2",
        className,
      )}
    >
      <code className="flex-1 overflow-x-auto whitespace-nowrap px-1 text-xs text-sky-300">
        {value}
      </code>
      <button
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 1200);
          } catch {
            /* clipboard may be blocked on insecure origins */
          }
        }}
        className="shrink-0 rounded-md p-1.5 text-slate-400 hover:bg-slate-800 hover:text-slate-200"
        title="Copy"
      >
        {copied ? <Check size={15} className="text-emerald-400" /> : <Copy size={15} />}
      </button>
    </div>
  );
}
