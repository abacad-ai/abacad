import { useLayoutEffect, useRef } from "react";
import { Link } from "react-router-dom";
import { ArrowRight, Layers, MoveRight, Plug, ShieldCheck } from "lucide-react";
import { PublicLayout } from "@/components/PublicLayout";
import { RotatingLockup } from "@/components/RotatingLockup";
import { AGENTS, DEVICES } from "@/components/brandLockups";
import { buttonVariants } from "@/components/ui/button";
import { useAuth } from "@/auth";
import { cn } from "@/lib/utils";

// Hero ceiling / floor in px. The headline scales down from HERO_MAX to keep the
// widest brand pairing on one line inside its column; below HERO_MIN it stops
// shrinking and wraps to two lines instead (phones), which stays readable.
const HERO_MAX = 60;
const HERO_MIN = 22;

// Keep the rotating headline on a single line by fitting the font to the column.
// The font can't be sized off the viewport: the text lives in a max-width column,
// and each rotating slot reserves the width of its *widest* brand (e.g. "Hermes
// Agent"), so the widest possible line — not the visible one — is what has to fit.
// We measure that widest line at the ceiling size and scale the font down until it
// fits the available width, dropping to two lines only when a phone is too narrow
// even at the floor size. Runs in a layout effect so the sizing is applied before
// paint (no flash), and re-runs on resize and once web fonts settle.
function useFitHeadline() {
  const boxRef = useRef<HTMLDivElement>(null);
  const h1Ref = useRef<HTMLHeadingElement>(null);

  useLayoutEffect(() => {
    const box = boxRef.current;
    const h1 = h1Ref.current;
    if (!box || !h1) return;

    const fit = () => {
      // Measure the natural single-line width at the ceiling size. The stacked
      // rotating slots make this width stable regardless of which brand shows.
      h1.style.whiteSpace = "nowrap";
      h1.style.fontSize = `${HERO_MAX}px`;
      const needed = h1.scrollWidth;
      const avail = box.clientWidth;

      let size = needed > avail ? Math.floor((avail / needed) * HERO_MAX) : HERO_MAX;
      if (size < HERO_MIN) {
        size = HERO_MIN;
        h1.style.whiteSpace = "normal"; // too tight for one line — let it wrap
      }
      h1.style.fontSize = `${size}px`;
    };

    fit();
    const ro = new ResizeObserver(fit);
    ro.observe(box);
    // Glyph advances shift once the display face loads; re-measure when it does.
    document.fonts?.ready.then(fit).catch(() => {});
    return () => ro.disconnect();
  }, []);

  return { boxRef, h1Ref };
}

// Public marketing homepage at `/`. Signed-out visitors get the pitch and a path
// into the console; signed-in visitors see an "Open console" shortcut instead of
// being bounced straight to a login screen.
export function LandingPage() {
  const { me } = useAuth();
  const { boxRef, h1Ref } = useFitHeadline();

  return (
    <PublicLayout>
      <div className="relative z-10 mx-auto flex w-full max-w-6xl flex-1 flex-col items-center px-4 py-16 text-center sm:px-6 sm:py-24">
        <span className="inline-flex items-center gap-2 rounded-full border border-border bg-surface/60 px-3 py-1.5 font-mono text-[11px] font-medium uppercase tracking-[0.14em] text-ink-muted">
          <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-success" aria-hidden="true" />
          works with any MCP agent
        </span>
        <div ref={boxRef} className="w-full">
          <h1
            ref={h1Ref}
            className="mt-6 text-balance font-display text-[clamp(1.05rem,5.4vw,3.75rem)] font-bold leading-[1.14] tracking-tight text-ink"
            aria-label="Connect any device to your coding agent."
          >
            Connect <RotatingLockup items={DEVICES} intervalMs={3000} /> to your{" "}
            <RotatingLockup items={AGENTS} intervalMs={3000} startDelayMs={1500} />.
          </h1>
        </div>
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
