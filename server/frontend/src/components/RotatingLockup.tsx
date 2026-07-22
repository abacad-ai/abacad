import { useEffect, useRef, useState } from "react";
import type { Brand } from "@/components/brandLockups";
import { cn } from "@/lib/utils";

// A single brand mark rendered inline. The SVG strings are static, sanitized
// assets from our own design tooling (see brandLockups.ts), so injecting them is
// safe; doing it this way keeps the official multi-path artwork byte-for-byte and
// lets each mark size to the surrounding font (height is set in `em`).
function BrandMark({ svg }: { svg: string }) {
  return (
    <span
      className="inline-flex items-center"
      style={{ lineHeight: 0 }}
      aria-hidden="true"
      dangerouslySetInnerHTML={{ __html: svg }}
    />
  );
}

// The icon + wordmark pair. Devices carry an indefinite article ("an Android")
// that rides along in the same rotating slot so the whole phrase swaps together.
function Lockup({ brand }: { brand: Brand }) {
  return (
    <span className="whitespace-nowrap">
      {brand.art ? <span className="text-ink">{brand.art} </span> : null}
      <BrandMark svg={brand.svg} />
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

  return (
    <span className="rot-slot">
      <span className={cn("rot-word", phase !== "idle" && phase)}>
        <Lockup brand={items[index]} />
      </span>
    </span>
  );
}
