import { Link } from "react-router-dom";
import { ArrowRight, Layers, MoveRight, Plug, ShieldCheck } from "lucide-react";
import { RelayMark } from "@/components/RelayMark";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

// Public marketing homepage at `/`. Signed-out visitors get the pitch and a path
// into the console; signed-in visitors see an "Open console" shortcut instead of
// being bounced straight to a login screen.
export function LandingPage() {
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
              <Link to="/devices" className={cn(buttonVariants({ size: "sm" }))}>
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

      <div className="relative z-10 mx-auto flex w-full max-w-6xl flex-1 flex-col items-center px-4 py-16 text-center sm:px-6 sm:py-24">
        <p className="font-mono text-[11px] font-medium uppercase tracking-[0.28em] text-brand">
          a device interface for agents
        </p>
        <h1 className="mt-5 max-w-3xl font-display text-4xl font-bold leading-[1.08] tracking-tight text-ink sm:text-6xl">
          Give your agent eyes and hands.
        </h1>
        <p className="mt-5 max-w-2xl text-base leading-7 text-ink-muted sm:text-lg">
          abacad turns a phone, Mac, or browser into a device an AI agent can see and control —
          once, from anywhere, with you supervising every step.
        </p>

        <div className="mt-9 flex flex-col items-center gap-3 sm:flex-row">
          {me ? (
            <Link to="/devices" className={cn(buttonVariants(), "px-5")}>
              Open console
              <ArrowRight size={17} />
            </Link>
          ) : (
            <>
              <Link to="/login" className={cn(buttonVariants(), "px-5")}>
                Get started
                <ArrowRight size={17} />
              </Link>
              <Link to="/login" className={cn(buttonVariants({ variant: "outline" }), "px-5")}>
                Sign in
              </Link>
            </>
          )}
        </div>

        {/* The core path: an agent reaches your devices through the relay. */}
        <div className="mt-12 flex items-center justify-center gap-2 font-mono text-[11px] uppercase tracking-[0.14em]">
          <span className="rounded border border-border bg-surface px-2.5 py-1.5 text-ink-muted">agent</span>
          <MoveRight size={15} className="shrink-0 text-brand" aria-hidden="true" />
          <span className="rounded border border-brand/25 bg-brand-soft px-2.5 py-1.5 text-brand">relay</span>
          <MoveRight size={15} className="shrink-0 text-brand" aria-hidden="true" />
          <span className="rounded border border-border bg-surface px-2.5 py-1.5 text-ink-muted">devices</span>
        </div>

        <div className="mt-16 grid w-full max-w-4xl gap-4 text-left sm:grid-cols-3">
          <Feature
            icon={Plug}
            title="One endpoint"
            body="Point your agent at a single MCP URL. abacad routes each command to the right online device."
          />
          <Feature
            icon={Layers}
            title="Every control surface"
            body="API, shell, accessibility tree, or screenshot — the agent picks the right rung for each action."
          />
          <Feature
            icon={ShieldCheck}
            title="You stay in control"
            body="A control tower, not a remote desktop. Approve sensitive actions and take over whenever you want."
          />
        </div>
      </div>

      <footer className="relative z-10 border-t border-border/70">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between px-4 font-mono text-[10px] uppercase tracking-[0.2em] text-ink-subtle sm:px-6">
          <span>abacad · device relay</span>
          <span>tokens hashed · secrets shown once</span>
        </div>
      </footer>
    </main>
  );
}

function Feature({
  icon: Icon,
  title,
  body,
}: {
  icon: typeof Plug;
  title: string;
  body: string;
}) {
  return (
    <div className="rounded-[10px] border border-border bg-surface/80 p-5 backdrop-blur">
      <span className="flex h-9 w-9 items-center justify-center rounded-md border border-brand/25 bg-brand-soft text-brand">
        <Icon size={17} />
      </span>
      <h3 className="mt-3.5 font-display text-[15px] font-bold text-ink">{title}</h3>
      <p className="mt-1.5 text-sm leading-6 text-ink-muted">{body}</p>
    </div>
  );
}
