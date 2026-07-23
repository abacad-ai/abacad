import { Link } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { type ReactNode } from "react";
import { RelayMark } from "@/components/RelayMark";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

// Chrome for the signed-out public pages (homepage, downloads). The header
// carries just the brand — no section nav and no auth buttons for signed-out
// visitors (the hero owns the single "Get started" call to action), so the
// public site stays one path into the console rather than a second navigation
// system competing with the dashboard's.
export function PublicLayout({ children }: { children: ReactNode }) {
  const { me } = useAuth();

  return (
    <main className="relative flex min-h-dvh flex-col overflow-hidden bg-canvas text-ink">
      <div className="bg-grid pointer-events-none absolute inset-0" aria-hidden="true" />
      <div className="glow-brand pointer-events-none absolute inset-0" aria-hidden="true" />

      <header className="relative z-10 border-b border-border/70">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-3 px-4 sm:px-6">
          <Link
            to="/"
            className="flex min-w-0 shrink-0 items-center gap-2.5 rounded-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-brand/30 bg-brand-soft text-brand">
              <RelayMark className="h-[22px] w-[22px]" />
            </span>
            <span className="font-display text-[15px] font-bold uppercase leading-none tracking-[0.22em] text-ink">
              abacad
            </span>
          </Link>

          {/* Signed-out visitors get their single "Get started" in the hero, so
              the header stays clean — only signed-in users get a console shortcut. */}
          {me ? (
            <div className="ml-auto flex items-center gap-2 sm:gap-3">
              <Link to="/" className={cn(buttonVariants({ size: "sm" }))}>
                Open console
                <ArrowRight size={16} />
              </Link>
            </div>
          ) : null}
        </div>
      </header>

      {children}

      <footer className="relative z-10 border-t border-border/70">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between gap-4 px-4 font-mono text-[10px] uppercase tracking-[0.2em] text-ink-subtle sm:px-6">
          <span>abacad · device relay</span>
          {/* /docs, /privacy, and /terms are server-rendered pages (not React
              routes), so they use plain anchors to force a full navigation rather
              than the client router's catch-all redirect. */}
          <nav className="flex items-center gap-4 sm:gap-5">
            <a href="/docs/" className="transition-colors hover:text-ink">
              Docs
            </a>
            <Link to="/downloads" className="transition-colors hover:text-ink">
              Downloads
            </Link>
            <a href="/privacy" className="transition-colors hover:text-ink">
              Privacy
            </a>
            <a href="/terms" className="transition-colors hover:text-ink">
              Terms
            </a>
          </nav>
        </div>
      </footer>
    </main>
  );
}
