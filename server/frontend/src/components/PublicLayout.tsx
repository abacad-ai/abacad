import { Link } from "react-router-dom";
import { ArrowRight } from "lucide-react";
import { type ReactNode } from "react";
import { RelayMark } from "@/components/RelayMark";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

// Chrome for the signed-out public pages (homepage, downloads). The header
// carries the brand and the sign-in actions only — no section nav, so the public
// site stays a single path into the console rather than a second navigation
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

          <div className="ml-auto flex items-center gap-2 sm:gap-3">
            {me ? (
              <Link to="/" className={cn(buttonVariants({ size: "sm" }))}>
                Open console
                <ArrowRight size={16} />
              </Link>
            ) : (
              <>
                <Link
                  to="/login"
                  className="rounded-md px-2.5 py-1.5 text-sm font-medium text-ink-muted transition-colors hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand sm:px-3"
                >
                  Sign in
                </Link>
                <Link to="/login" className={cn(buttonVariants({ size: "sm" }))}>
                  Get started
                  <ArrowRight size={16} />
                </Link>
              </>
            )}
          </div>
        </div>
      </header>

      {children}

      <footer className="relative z-10 border-t border-border/70">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between gap-4 px-4 font-mono text-[10px] uppercase tracking-[0.2em] text-ink-subtle sm:px-6">
          <span>abacad · device relay</span>
          <Link to="/downloads" className="transition-colors hover:text-ink">
            Downloads
          </Link>
        </div>
      </footer>
    </main>
  );
}
