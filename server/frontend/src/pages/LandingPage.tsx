import { Link } from "react-router-dom";
import { ArrowRight, Layers, MoveRight, Plug, ShieldCheck } from "lucide-react";
import { PublicLayout } from "@/components/PublicLayout";
import { RotatingLockup } from "@/components/RotatingLockup";
import { AGENTS, DEVICES } from "@/components/brandLockups";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

// Public marketing homepage at `/`. Signed-out visitors get the pitch and a path
// into the console; signed-in visitors see an "Open console" shortcut instead of
// being bounced straight to a login screen.
export function LandingPage() {
  const { me } = useAuth();

  return (
    <PublicLayout>
      <div className="relative z-10 mx-auto flex w-full max-w-6xl flex-1 flex-col items-center px-4 py-16 text-center sm:px-6 sm:py-24">
        <span className="inline-flex items-center gap-2 rounded-full border border-border bg-surface/60 px-3 py-1.5 font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-ink-muted">
          <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-success" aria-hidden="true" />
          works with any MCP agent
        </span>
        <h1
          className="mt-6 whitespace-nowrap font-display text-[clamp(1.05rem,5.4vw,3.75rem)] font-bold leading-[1.14] tracking-tight text-ink"
          aria-label="Connect any device to your coding agent."
        >
          Connect <RotatingLockup items={DEVICES} intervalMs={3000} /> to your{" "}
          <RotatingLockup items={AGENTS} intervalMs={3000} startDelayMs={1500} />.
        </h1>
        <p className="mt-6 max-w-2xl text-base leading-7 text-ink-muted sm:text-lg">
          Connect a phone, laptop, or browser as a device — then point your coding agent at one
          endpoint and let it drive, with you approving every step.
        </p>

        <div className="mt-9 flex flex-col items-center gap-3 sm:flex-row">
          {me ? (
            <Link to="/" className={cn(buttonVariants(), "px-5")}>
              Open console
              <ArrowRight size={17} />
            </Link>
          ) : (
            <Link to="/login" className={cn(buttonVariants(), "px-5")}>
              Get started
              <ArrowRight size={17} />
            </Link>
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
    </PublicLayout>
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
