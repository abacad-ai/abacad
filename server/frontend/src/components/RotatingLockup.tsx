import { useEffect, useRef, useState } from "react";
import type { Brand } from "@/components/brandLockups";
import { cn } from "@/lib/utils";

// A single brand mark rendered inline. The SVG strings are static, sanitized
// assets vendored verbatim from thesvg.org (see brandLockups.ts), so injecting
// them is safe. The markup is untouched: `.brand-mark` sizes it to the font and
// handles light/dark colour externally, and `.mono` maps a black/white mark to
// the current text colour — none of it edits the artwork.
function BrandMark({ brand }: { brand: Brand }) {
  return (
    <span
      className={cn("brand-mark", brand.mono && "mono")}
      dangerouslySetInnerHTML={{ __html: brand.svg }}
    />
  );
}

// The icon + wordmark pair. Devices carry an indefinite article ("an Android")
// that rides along in the same rotating slot so the whole phrase swaps together.
function Lockup({ brand }: { brand: Brand }) {
  return (
    <span className="whitespace-nowrap">
      {brand.art ? <span className="text-ink">{brand.art} </span> : null}
      <BrandMark brand={brand} />
      <span className="font-bold text-ink"> {brand.name}</span>
    </span>
  );
}

function usePrefersReducedMotion() {
  const [reduced, setReduced] = useState(
    () =>
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches,
  );
  useEffect(() => {
    const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
    const onChange = () => setReduced(mq.matches);
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  return reduced;
}

// Cycles through `items`, swapping the visible lockup with a fade + slide + blur.
// Devices and agents rotate on their own timers (and a start offset) so the
// pairings keep shuffling. Under prefers-reduced-motion it holds the first item.
export function RotatingLockup({
  items,
  intervalMs,
  startDelayMs = 0,
}: {
  items: Brand[];
  intervalMs: number;
  startDelayMs?: number;
}) {
  const reduced = usePrefersReducedMotion();
  const [index, setIndex] = useState(0);
  const [phase, setPhase] = useState<"idle" | "leave" | "enter">("idle");
  const swapTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (reduced) return;
    let intervalId: number;
    const startId = window.setTimeout(() => {
      intervalId = window.setInterval(() => {
        setPhase("leave");
        swapTimer.current = window.setTimeout(() => {
          setIndex((i) => (i + 1) % items.length);
          setPhase("enter");
          // Two frames so the browser paints the entering start state before we
          // transition it home — otherwise the "enter" step is skipped.
          requestAnimationFrame(() =>
            requestAnimationFrame(() => setPhase("idle")),
          );
        }, 300);
      }, intervalMs);
    }, startDelayMs);
    return () => {
      window.clearTimeout(startId);
      window.clearInterval(intervalId);
      window.clearTimeout(swapTimer.current);
    };
  }, [reduced, items.length, intervalMs, startDelayMs]);

  // Every item is stacked in one grid cell, so the slot stays as wide as the
  // widest lockup and the words around it never reflow. Only the active item is
  // visible; it fades/slides/blurs on each swap. Inlining each icon exactly once
  // keeps SVGs with internal ids (Gemini, Linux) from colliding. Hidden from the
  // a11y tree — the <h1> carries the real, static label.
  return (
    <span className="rot-slot" aria-hidden="true">
      {items.map((brand, i) => (
        <span
          key={i}
          className={cn("rot-item", i === index && (phase === "idle" ? "active" : phase))}
        >
          <Lockup brand={brand} />
        </span>
      ))}
    </span>
  );
}
